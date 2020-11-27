package consensus

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	msg_pb "github.com/harmony-one/harmony/api/proto/message"
	"github.com/harmony-one/harmony/consensus/signature"
	"github.com/harmony-one/harmony/core/types"
	nodeconfig "github.com/harmony-one/harmony/internal/configs/node"
	"github.com/harmony-one/harmony/internal/utils"
	"github.com/harmony-one/harmony/p2p"
	"github.com/klauspost/reedsolomon"
)

func (consensus *Consensus) onAnnounce(msg *msg_pb.Message) {
	recvMsg, err := ParseFBFTMessage(msg)
	if err != nil {
		consensus.getLogger().Error().
			Err(err).
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Msg("[OnAnnounce] Unparseable leader message")
		return
	}

	// NOTE let it handle its own logs
	if !consensus.onAnnounceSanityChecks(recvMsg) {
		return
	}

	consensus.getLogger().Debug().
		Uint64("MsgViewID", recvMsg.ViewID).
		Uint64("MsgBlockNum", recvMsg.BlockNum).
		Msg("[OnAnnounce] Announce message Added")
	consensus.FBFTLog.AddMessage(recvMsg)
	consensus.mutex.Lock()
	defer consensus.mutex.Unlock()
	consensus.blockHash = recvMsg.BlockHash
	// we have already added message and block, skip check viewID
	// and send prepare message if is in ViewChanging mode
	if consensus.IsViewChangingMode() {
		consensus.getLogger().Debug().
			Msg("[OnAnnounce] Still in ViewChanging Mode, Exiting !!")
		return
	}

	if consensus.checkViewID(recvMsg) != nil {
		if consensus.current.Mode() == Normal {
			consensus.getLogger().Debug().
				Uint64("MsgViewID", recvMsg.ViewID).
				Uint64("MsgBlockNum", recvMsg.BlockNum).
				Msg("[OnAnnounce] ViewID check failed")
		}
		return
	}
	consensus.StartFinalityCount()
	consensus.prepare()

	consensus.globalBlockSlice = make([][]byte, 13)
	consensus.globalBlockSliceCnt = 0
}

func (consensus *Consensus) prepare() {
	groupID := []nodeconfig.GroupID{nodeconfig.NewGroupIDByShardID(nodeconfig.ShardID(consensus.ShardID))}
	for _, key := range consensus.priKey {
		if !consensus.IsValidatorInCommittee(key.Pub.Bytes) {
			continue
		}
		networkMessage, err := consensus.construct(msg_pb.MessageType_PREPARE, nil, &key)
		if err != nil {
			consensus.getLogger().Err(err).
				Str("message-type", msg_pb.MessageType_PREPARE.String()).
				Msg("could not construct message")
			return
		}

		// TODO: this will not return immediately, may block
		if consensus.current.Mode() != Listening {
			if err := consensus.msgSender.SendWithoutRetry(
				groupID,
				p2p.ConstructMessage(networkMessage.Bytes),
			); err != nil {
				consensus.getLogger().Warn().Err(err).Msg("[OnAnnounce] Cannot send prepare message")
			} else {
				consensus.getLogger().Info().
					Str("blockHash", hex.EncodeToString(consensus.blockHash[:])).
					Msg("[OnAnnounce] Sent Prepare Message!!")
			}
		}
	}
	consensus.getLogger().Debug().
		Str("From", consensus.phase.String()).
		Str("To", FBFTPrepare.String()).
		Msg("[Announce] Switching Phase")
	consensus.switchPhase(FBFTPrepare, true)
}

// if onPrepared accepts the prepared message from the leader, then
// it will send a COMMIT message for the leader to receive on the network.
func (consensus *Consensus) onPrepared(msg *msg_pb.Message) {
	recvMsg, err := ParseFBFTMessage(msg)
	if err != nil {
		consensus.getLogger().Debug().Err(err).Msg("[OnPrepared] Unparseable validator message")
		return
	}
	consensus.getLogger().Info().
		Uint64("MsgBlockNum", recvMsg.BlockNum).
		Uint64("MsgViewID", recvMsg.ViewID).
		Msg("[OnPrepared] Received prepared message")

	if recvMsg.BlockNum < consensus.blockNum {
		consensus.getLogger().Debug().Uint64("MsgBlockNum", recvMsg.BlockNum).
			Msg("Wrong BlockNum Received, ignoring!")
		return
	}

	// check validity of prepared signature
	blockHash := recvMsg.BlockHash
	aggSig, mask, err := consensus.ReadSignatureBitmapPayload(recvMsg.Payload, 0)
	if err != nil {
		consensus.getLogger().Error().Err(err).Msg("ReadSignatureBitmapPayload failed!")
		return
	}

	if !consensus.Decider.IsQuorumAchievedByMask(mask) {
		consensus.getLogger().Warn().
			Msgf("[OnPrepared] Quorum Not achieved")
		return
	}

	if !aggSig.VerifyHash(mask.AggregatePublic, blockHash[:]) {
		myBlockHash := common.Hash{}
		myBlockHash.SetBytes(consensus.blockHash[:])
		consensus.getLogger().Warn().
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Uint64("MsgViewID", recvMsg.ViewID).
			Msg("[OnPrepared] failed to verify multi signature for prepare phase")
		return
	}
	// lyn 额外判断是否收到了旧的prepare
	if recvMsg.BlockNum < consensus.blockNum {
		consensus.getLogger().Debug().
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Uint64("blockNum", consensus.blockNum).
			Msg("[OnPrepared] before Block Received, ignoring!!")
		return
	}
	// check validity of block
	var blockObj types.Block
	// lyn log 打印出现在收到的块片段数据
	// utils.Logger().Info().Str("-------------------", string(recvMsg.Block)).
	// 	Msg("喵喵 DecodeBytes")

	totalLen := BytesToInt(recvMsg.Block[0:8])     //原始块长度
	comingIndex := BytesToInt(recvMsg.Block[8:16]) //来的片段的下标
	//已经拼接过了block，不需要再收信息了
	if consensus.globalBlockSliceCnt == 10 {
		return
	}
	// 没来过的片才会往里加
	if consensus.globalBlockSlice[comingIndex] == nil {
		consensus.globalBlockSlice[comingIndex] = recvMsg.Block[16:]
		consensus.globalBlockSliceCnt = consensus.globalBlockSliceCnt + 1
		utils.Logger().Info().Int("totalLen", totalLen).Int("comingIndex", comingIndex).
			Msg("here is one slice of block")

		//没有达到10个片则不作额外验证，等着收其他的片
		if consensus.globalBlockSliceCnt != 10 {
			return
		} else {
			//拼接
			utils.Logger().Info().
				Msg("现在收到了10个片 尝试拼接")
			b1 := new(bytes.Buffer)
			// if consensus.globalBlockSlice[9] == nil {
			// 	utils.Logger().Info().
			// 		Msg("before100000000000000000 nillllllllllll")
			// 	// return
			// }
			enc, err := reedsolomon.New(10, 3)
			err = enc.ReconstructData(consensus.globalBlockSlice)
			// if consensus.globalBlockSlice[9] == nil {
			// 	utils.Logger().Info().
			// 		Msg("after100000000000000000 nillllllllllll")
			// 	// return
			// }
			if err != nil {
				utils.Logger().Info().
					Msg("拼接出现错误！！ReconstructData")
				return
			}
			err = enc.Join(b1, consensus.globalBlockSlice, totalLen)
			readBuf, _ := ioutil.ReadAll(b1)
			recvMsg.Block = readBuf
			if err != nil {
				utils.Logger().Info().
					Msg("拼接出现错误！！一般不会出现这个情况！")
				return
			}

		}
	}

	if err := rlp.DecodeBytes(recvMsg.Block, &blockObj); err != nil {
		consensus.getLogger().Warn().
			Err(err).
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Msg("[OnPrepared] Unparseable block header data")
		return
	}

	// 我改了
	// let this handle it own logs
	if !consensus.onPreparedSanityChecks(&blockObj, recvMsg) {
		return
	}
	consensus.mutex.Lock()
	defer consensus.mutex.Unlock()

	consensus.FBFTLog.AddBlock(&blockObj)
	// add block field
	blockPayload := make([]byte, len(recvMsg.Block))
	copy(blockPayload[:], recvMsg.Block[:])
	consensus.block = blockPayload
	recvMsg.Block = []byte{} // save memory space
	consensus.FBFTLog.AddMessage(recvMsg)
	consensus.getLogger().Debug().
		Uint64("MsgViewID", recvMsg.ViewID).
		Uint64("MsgBlockNum", recvMsg.BlockNum).
		Hex("blockHash", recvMsg.BlockHash[:]).
		Msg("[OnPrepared] Prepared message and block added")

	// tryCatchup is also run in onCommitted(), so need to lock with commitMutex.
	consensus.tryCatchup()

	if consensus.current.Mode() != Normal {
		// don't sign the block that is not verified
		consensus.getLogger().Info().Msg("[OnPrepared] Not in normal mode, Exiting!!")
		return
	}

	if consensus.checkViewID(recvMsg) != nil {
		if consensus.current.Mode() == Normal {
			consensus.getLogger().Debug().
				Uint64("MsgViewID", recvMsg.ViewID).
				Uint64("MsgBlockNum", recvMsg.BlockNum).
				Msg("[OnPrepared] ViewID check failed")
		}
		return
	}
	if recvMsg.BlockNum > consensus.blockNum {
		consensus.getLogger().Debug().
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Uint64("blockNum", consensus.blockNum).
			Msg("[OnPrepared] Future Block Received, ignoring!!")
		return
	}

	// TODO: genesis account node delay for 1 second,
	// this is a temp fix for allows FN nodes to earning reward
	if consensus.delayCommit > 0 {
		time.Sleep(consensus.delayCommit)
	}

	// add preparedSig field
	consensus.aggregatedPrepareSig = aggSig
	consensus.prepareBitmap = mask

	// Optimistically add blockhash field of prepare message
	emptyHash := [32]byte{}
	if bytes.Equal(consensus.blockHash[:], emptyHash[:]) {
		copy(consensus.blockHash[:], blockHash[:])
	}

	// Sign commit signature on the received block
	commitPayload := signature.ConstructCommitPayload(consensus.ChainReader,
		blockObj.Epoch(), blockObj.Hash(), blockObj.NumberU64(), blockObj.Header().ViewID().Uint64())
	groupID := []nodeconfig.GroupID{
		nodeconfig.NewGroupIDByShardID(nodeconfig.ShardID(consensus.ShardID)),
	}
	for _, key := range consensus.priKey {
		if !consensus.IsValidatorInCommittee(key.Pub.Bytes) {
			continue
		}

		networkMessage, _ := consensus.construct(
			msg_pb.MessageType_COMMIT,
			commitPayload,
			&key,
		)

		if consensus.current.Mode() != Listening {
			if err := consensus.msgSender.SendWithoutRetry(
				groupID,
				p2p.ConstructMessage(networkMessage.Bytes),
			); err != nil {
				consensus.getLogger().Warn().Msg("[OnPrepared] Cannot send commit message!!")
			} else {
				consensus.getLogger().Info().
					Uint64("blockNum", consensus.blockNum).
					Hex("blockHash", consensus.blockHash[:]).
					Msg("[OnPrepared] Sent Commit Message!!")
			}
		}
	}
	consensus.getLogger().Debug().
		Str("From", consensus.phase.String()).
		Str("To", FBFTCommit.String()).
		Msg("[OnPrepared] Switching phase")

	consensus.switchPhase(FBFTCommit, true)
}

func (consensus *Consensus) onCommitted(msg *msg_pb.Message) {
	recvMsg, err := ParseFBFTMessage(msg)
	if err != nil {
		consensus.getLogger().Warn().Msg("[OnCommitted] unable to parse msg")
		return
	}
	// NOTE let it handle its own logs
	if !consensus.isRightBlockNumCheck(recvMsg) {
		return
	}

	aggSig, mask, err := consensus.ReadSignatureBitmapPayload(recvMsg.Payload, 0)
	if err != nil {
		consensus.getLogger().Error().Err(err).Msg("[OnCommitted] readSignatureBitmapPayload failed")
		return
	}

	if !consensus.Decider.IsQuorumAchievedByMask(mask) {
		consensus.getLogger().Warn().
			Msgf("[OnCommitted] Quorum Not achieved")
		return
	}

	// Must have the corresponding block to verify committed message.
	blockObj := consensus.FBFTLog.GetBlockByHash(recvMsg.BlockHash)
	if blockObj == nil {
		consensus.getLogger().Debug().
			Uint64("blockNum", recvMsg.BlockNum).
			Uint64("viewID", recvMsg.ViewID).
			Str("blockHash", recvMsg.BlockHash.Hex()).
			Msg("[OnCommitted] Failed finding a matching block for committed message")
		return
	}
	commitPayload := signature.ConstructCommitPayload(consensus.ChainReader,
		blockObj.Epoch(), blockObj.Hash(), blockObj.NumberU64(), blockObj.Header().ViewID().Uint64())
	if !aggSig.VerifyHash(mask.AggregatePublic, commitPayload) {
		consensus.getLogger().Error().
			Uint64("MsgBlockNum", recvMsg.BlockNum).
			Msg("[OnCommitted] Failed to verify the multi signature for commit phase")
		return
	}

	consensus.FBFTLog.AddMessage(recvMsg)

	consensus.mutex.Lock()
	defer consensus.mutex.Unlock()

	consensus.aggregatedCommitSig = aggSig
	consensus.commitBitmap = mask

	if recvMsg.BlockNum > consensus.blockNum && recvMsg.BlockNum-consensus.blockNum > consensusBlockNumBuffer {
		consensus.getLogger().Info().Uint64("MsgBlockNum", recvMsg.BlockNum).Msg("[OnCommitted] OUT OF SYNC")
		go func() {
			select {
			case consensus.BlockNumLowChan <- struct{}{}:
				consensus.current.SetMode(Syncing)
				for _, v := range consensus.consensusTimeout {
					v.Stop()
				}
			case <-time.After(1 * time.Second):
			}
		}()
		return
	}

	consensus.tryCatchup()
	if consensus.IsViewChangingMode() {
		consensus.getLogger().Info().Msg("[OnCommitted] Still in ViewChanging mode, Exiting!!")
		return
	}

	if consensus.consensusTimeout[timeoutBootstrap].IsActive() {
		consensus.consensusTimeout[timeoutBootstrap].Stop()
		consensus.getLogger().Debug().Msg("[OnCommitted] Start consensus timer; stop bootstrap timer only once")
	} else {
		consensus.getLogger().Debug().Msg("[OnCommitted] Start consensus timer")
	}
	consensus.consensusTimeout[timeoutConsensus].Start()
}
