package consensus

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/rlp"
	msg_pb "github.com/harmony-one/harmony/api/proto/message"
	"github.com/harmony-one/harmony/consensus/quorum"
	"github.com/harmony-one/harmony/consensus/signature"
	"github.com/harmony-one/harmony/core/types"
	nodeconfig "github.com/harmony-one/harmony/internal/configs/node"
	"github.com/harmony-one/harmony/internal/utils"
	"github.com/harmony-one/harmony/p2p"
	"github.com/klauspost/reedsolomon"
)

// lyn 加入的用于int 与byte 转化
// IntToBytes ...
func IntToBytes(n int) []byte {
	data := int64(n)
	bytebuf := bytes.NewBuffer([]byte{})
	binary.Write(bytebuf, binary.BigEndian, data)
	return bytebuf.Bytes()
}

// BytesToInt ...
func BytesToInt(bys []byte) int {
	bytebuff := bytes.NewBuffer(bys)
	var data int64
	binary.Read(bytebuff, binary.BigEndian, &data)
	return int(data)
}

func (consensus *Consensus) didReachPrepareQuorum() error {
	logger := utils.Logger()
	logger.Info().Msg("[OnPrepare] Received Enough Prepare Signatures")
	leaderPriKey, err := consensus.GetConsensusLeaderPrivateKey()
	if err != nil {
		utils.Logger().Warn().Err(err).Msg("[OnPrepare] leader not found")
		return err
	}
	// Construct and broadcast prepared message
	networkMessage, err := consensus.construct(
		msg_pb.MessageType_PREPARED, nil, leaderPriKey,
	)
	if err != nil {
		consensus.getLogger().Err(err).
			Str("message-type", msg_pb.MessageType_PREPARED.String()).
			Msg("failed constructing message")
		return err
	}
	// lyn 这里的msgToSend 不需要使用
	_, FBFTMsg, aggSig :=
		networkMessage.Bytes,
		networkMessage.FBFTMsg,
		networkMessage.OptionalAggregateSignature

	consensus.aggregatedPrepareSig = aggSig
	consensus.FBFTLog.AddMessage(FBFTMsg)
	// Leader add commit phase signature
	var blockObj types.Block
	if err := rlp.DecodeBytes(consensus.block, &blockObj); err != nil {
		consensus.getLogger().Warn().
			Err(err).
			Uint64("BlockNum", consensus.blockNum).
			Msg("[didReachPrepareQuorum] Unparseable block data")
		return err
	}
	commitPayload := signature.ConstructCommitPayload(consensus.ChainReader,
		blockObj.Epoch(), blockObj.Hash(), blockObj.NumberU64(), blockObj.Header().ViewID().Uint64())

	// so by this point, everyone has committed to the blockhash of this block
	// in prepare and so this is the actual block.
	for i, key := range consensus.priKey {
		if err := consensus.commitBitmap.SetKey(key.Pub.Bytes, true); err != nil {
			consensus.getLogger().Warn().Msgf("[OnPrepare] Leader commit bitmap set failed for key at index %d", i)
			continue
		}

		if _, err := consensus.Decider.AddNewVote(
			quorum.Commit,
			key.Pub.Bytes,
			key.Pri.SignHash(commitPayload),
			blockObj.Hash(),
			blockObj.NumberU64(),
			blockObj.Header().ViewID().Uint64(),
		); err != nil {
			return err
		}
	}

	// utils.Logger().Info().
	// 	Msg(" *******************************************")
	// enc, err := reedsolomon.New(10, 3)
	// // data := make([][]byte, 13)
	// // // Create all shards, size them at 50000 each
	// // for i := range data {
	// // 	data[i] = make([]byte, 50000)
	// // }
	// data := make([]byte, 100)
	// // Fill some data into the data shards
	// for i := range data {
	// 	data[i] = byte((i) & 0xff)

	// }
	// for _, in := range data {
	// 	utils.Logger().Info().Int("data", int(in)).
	// 		Msg(" *******11111")
	// }
	// split, err := enc.Split(data)

	// for _, in := range split {
	// 	utils.Logger().Info().Int("data", BytesToInt(in)).
	// 		Msg(" *******22222")
	// }
	// split[1] = nil
	// for _, in := range split {
	// 	utils.Logger().Info().Int("data", BytesToInt(in)).
	// 		Msg(" *******33333")
	// }
	// b1 := new(bytes.Buffer)

	// err := enc.Reconstruct(split)
	// err = enc.Join(b1, split, 100)
	// readBuf, _ := ioutil.ReadAll(b1)

	// // err = enc.Encode(data)
	// for _, in := range readBuf {
	// 	utils.Logger().Info().Int("data", int(in)).
	// 		Msg(" *******44444")
	// }
	// Delete two data shards
	// data[3] = nil
	// data[7] = nil

	// Reconstruct the missing shards
	// err = enc.Reconstruct(data)
	// for _, in := range data {
	// 	utils.Logger().Info().Int("data", BytesToInt(in)).
	// 		Msg(" *******333333")
	// }
	// if err != nil {
	// 	utils.Logger().Info().
	// 		Msg("erasure-code过程出错 join函数出现了错误返回")
	// }
	// utils.Logger().Info().
	// 	Msg(" *******************************************")

	// lyn here 切分block
	utils.Logger().Info().
		Msg("______________leader尝试切分数据")
	// utils.Logger().Info().Int("喵喵,原始block数据长度", len(consensus.block)).Msg("长度")
	enc, err := reedsolomon.New(SliceCnt, ParityCnt)
	split, err := enc.Split(consensus.block)
	err = enc.Encode(split)

	// 直接合并出原始数据用于测试
	b1 := new(bytes.Buffer)
	err = enc.Join(b1, split, len(consensus.block))
	if err != nil {
		utils.Logger().Info().
			Msg("erasure-code过程出错 join函数出现了错误返回")
	}
	readbuf, _ := ioutil.ReadAll(b1)
	if len(readbuf) != len(consensus.block) {
		utils.Logger().Info().
			Msg("______________长度不一样")
	} else {
		for index := range readbuf {
			if readbuf[index] != consensus.block[index] {
				utils.Logger().Info().
					Msg("______________内容不一样")
			}
		}
	}
	for index, thing := range split {
		// 这里需要额外加 8 + 8 个 byte 用于储存 total size + index
		s := make([][]byte, 3)
		s[0] = IntToBytes(len(consensus.block))
		s[1] = IntToBytes(index)
		s[2] = thing

		sep := bytes.Join(s, []byte{})
		utils.Logger().Info().Int("—!!!!", BytesToInt(sep[0:8])).
			Msg("喵喵 切片数据长度")
		utils.Logger().Info().Int("—!!!!", BytesToInt(sep[8:16])).
			Msg("喵喵 切片数据index")
		networkMessage, err = consensus.construct(
			msg_pb.MessageType_PREPARED, nil, leaderPriKey, sep,
		)
		if err != nil {
			consensus.getLogger().Err(err).
				Str("message-type", msg_pb.MessageType_PREPARED.String()).
				Msg("failed constructing message")
			return err
		}
		if err := consensus.msgSender.SendWithRetry(
			consensus.blockNum,
			msg_pb.MessageType_PREPARED, []nodeconfig.GroupID{
				nodeconfig.NewGroupIDByShardID(nodeconfig.ShardID(consensus.ShardID)),
			},
			p2p.ConstructMessage(networkMessage.Bytes),
		); err != nil {
			consensus.getLogger().Warn().Msg("[OnPrepare] Cannot send prepared message")
		} else {
			consensus.getLogger().Info().
				Hex("blockHash", consensus.blockHash[:]).
				Uint64("blockNum", consensus.blockNum).
				Msg("[OnPrepare] Sent Prepared Message!!")
			// lyn log 切片数据内容
			// utils.Logger().Info().Str("——————————————", string(sep)).
			// 	Msg("喵喵 Prepared Messageblock is ！")
		}
	}
	consensus.msgSender.StopRetry(msg_pb.MessageType_ANNOUNCE)
	// Stop retry committed msg of last consensus
	consensus.msgSender.StopRetry(msg_pb.MessageType_COMMITTED)

	consensus.getLogger().Debug().
		Str("From", consensus.phase.String()).
		Str("To", FBFTCommit.String()).
		Msg("[OnPrepare] Switching phase")

	return nil
}
