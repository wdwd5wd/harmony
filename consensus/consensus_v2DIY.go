package consensus

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync/atomic"
	"time"

	msg_pb "github.com/harmony-one/harmony/api/proto/message"
	"github.com/harmony-one/harmony/consensus/quorum"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/crypto/bls"
	nodeconfig "github.com/harmony-one/harmony/internal/configs/node"
	"github.com/harmony-one/harmony/p2p"
)

// HandleMessageUpdate will update the consensus state according to received message
func (consensus *Consensus) HandleMessageUpdateDIY(ctx context.Context, msg *msg_pb.Message, senderKey *bls.SerializedPublicKey) error {
	// when node is in ViewChanging mode, it still accepts normal messages into FBFTLog
	// in order to avoid possible trap forever but drop PREPARE and COMMIT
	// which are message types specifically for a node acting as leader
	// so we just ignore those messages
	if consensus.IsViewChangingMode() &&
		(msg.Type == msg_pb.MessageType_PREPARE ||
			msg.Type == msg_pb.MessageType_COMMIT) {
		return nil
	}

	intendedForValidator, intendedForLeader :=
		!consensus.IsLeader(),
		consensus.IsLeader()

	switch t := msg.Type; true {
	// Handle validator intended messages first
	case t == msg_pb.MessageType_ANNOUNCE && intendedForValidator:
		if !bytes.Equal(senderKey[:], consensus.LeaderPubKey.Bytes[:]) &&
			consensus.current.Mode() == Normal && !consensus.IgnoreViewIDCheck.IsSet() {
			return errSenderPubKeyNotLeader
		}
		if !consensus.senderKeySanityChecks(msg, senderKey) {
			return errVerifyMessageSignature
		}
		consensus.onAnnounceDIY(msg)

		// for {
		// 	if msg.GetConsensus().BlockNum == 1 {
		// 		consensus.onAnnounceDIY(msg)
		// 		break
		// 	} else {
		// 		consensus.getLogger().Debug().Msg("RECEIVING commitFinishSig")
		// 		select {
		// 		case <-consensus.commitFinishSig:
		// 			consensus.getLogger().Debug().Msg("RECEIVED commitFinishSig")
		// 			consensus.onAnnounceDIY(msg)
		// 			break
		// 		}
		// 	}
		// }

	// case t == msg_pb.MessageType_PREPARED && intendedForValidator:
	// 	if !bytes.Equal(senderKey[:], consensus.LeaderPubKey.Bytes[:]) &&
	// 		consensus.current.Mode() == Normal && !consensus.IgnoreViewIDCheck.IsSet() {
	// 		return errSenderPubKeyNotLeader
	// 	}
	// 	if !consensus.senderKeySanityChecks(msg, senderKey) {
	// 		return errVerifyMessageSignature
	// 	}
	// 	consensus.onPrepared(msg)
	case t == msg_pb.MessageType_COMMITTED && intendedForValidator:
		if !bytes.Equal(senderKey[:], consensus.LeaderPubKey.Bytes[:]) &&
			consensus.current.Mode() == Normal && !consensus.IgnoreViewIDCheck.IsSet() {
			return errSenderPubKeyNotLeader
		}
		if !consensus.senderKeySanityChecks(msg, senderKey) {
			return errVerifyMessageSignature
		}
		consensus.onCommittedDIY(msg)

	// Handle leader intended messages now
	// case t == msg_pb.MessageType_PREPARE && intendedForLeader:
	// 	consensus.onPrepare(msg)
	case t == msg_pb.MessageType_COMMIT && intendedForLeader:
		consensus.onCommitDIY(msg)

		// Handle view change messages
	case t == msg_pb.MessageType_VIEWCHANGE:
		if !consensus.senderKeySanityChecks(msg, senderKey) {
			return errVerifyMessageSignature
		}
		consensus.onViewChange(msg)
	case t == msg_pb.MessageType_NEWVIEW:
		if !consensus.senderKeySanityChecks(msg, senderKey) {
			return errVerifyMessageSignature
		}
		consensus.onNewView(msg)
	}

	return nil
}

// try to catch up if fall behind
func (consensus *Consensus) tryCatchupDIY() {
	consensus.getLogger().Info().Msg("[TryCatchup] commit new blocks")
	currentBlockNum := consensus.blockNum
	for {
		msgs := consensus.FBFTLog.GetMessagesByTypeSeq(
			msg_pb.MessageType_COMMITTED, consensus.blockNum,
		)
		if len(msgs) == 0 {
			break
		}
		if len(msgs) > 1 {
			consensus.getLogger().Error().
				Int("numMsgs", len(msgs)).
				Msg("[TryCatchup] DANGER!!! we should only get one committed message for a given blockNum")
		}

		var committedMsg *FBFTMessage
		var block *types.Block
		for i := range msgs {
			tmpBlock := consensus.FBFTLog.GetBlockByHash(msgs[i].BlockHash)
			if tmpBlock == nil {
				blksRepr, msgsRepr, incomingMsg :=
					consensus.FBFTLog.Blocks().String(),
					consensus.FBFTLog.Messages().String(),
					msgs[i].String()
				consensus.getLogger().Debug().
					Str("FBFT-log-blocks", blksRepr).
					Str("FBFT-log-messages", msgsRepr).
					Str("incoming-message", incomingMsg).
					Uint64("blockNum", msgs[i].BlockNum).
					Uint64("viewID", msgs[i].ViewID).
					Str("blockHash", msgs[i].BlockHash.Hex()).
					Msg("[TryCatchup] Failed finding a matching block for committed message")
				continue
			}

			committedMsg = msgs[i]
			block = tmpBlock
			break
		}
		if block == nil || committedMsg == nil {
			consensus.getLogger().Error().Msg("[TryCatchup] Failed finding a valid committed message.")
			break
		}

		// if block.ParentHash() != consensus.ChainReader.CurrentHeader().Hash() {
		// 	consensus.getLogger().Debug().Msg("[TryCatchup] parent block hash not match")
		// 	break
		// }
		consensus.getLogger().Info().Msg("[TryCatchup] block found to commit")

		preparedMsgs := consensus.FBFTLog.GetMessagesByTypeSeqHash(
			msg_pb.MessageType_ANNOUNCE, committedMsg.BlockNum, committedMsg.BlockHash,
		)
		msg := consensus.FBFTLog.FindMessageByMaxViewID(preparedMsgs)
		if msg == nil {
			break
		}
		consensus.getLogger().Info().Msg("[TryCatchup] prepared message found to commit")

		atomic.AddUint64(&consensus.blockNum, 1)
		consensus.SetCurBlockViewID(committedMsg.ViewID + 1)

		consensus.LeaderPubKey = committedMsg.SenderPubkey

		consensus.getLogger().Info().Msg("[TryCatchup] Adding block to chain")

		// Fill in the commit signatures
		block.SetCurrentCommitSig(committedMsg.Payload)
		consensus.OnConsensusDone(block)
		consensus.ResetState()

		select {
		case consensus.VerifiedNewBlock <- block:
		default:
			consensus.getLogger().Info().
				Str("blockHash", block.Hash().String()).
				Msg("[TryCatchup] consensus verified block send to chan failed")
			continue
		}

		break
	}
	if currentBlockNum < consensus.blockNum {
		consensus.getLogger().Info().
			Uint64("From", currentBlockNum).
			Uint64("To", consensus.blockNum).
			Msg("[TryCatchup] Caught up!")
		consensus.switchPhase(FBFTAnnounce, true)
	}
	// catup up and skip from view change trap
	if currentBlockNum < consensus.blockNum &&
		consensus.IsViewChangingMode() {
		consensus.current.SetMode(Normal)
		consensus.consensusTimeout[timeoutViewChange].Stop()
	}
	// clean up old log
	consensus.FBFTLog.DeleteBlocksLessThan(consensus.blockNum - 1)
	consensus.FBFTLog.DeleteMessagesLessThan(consensus.blockNum - 1)
}

func (consensus *Consensus) finalizeCommitsDIY() {
	consensus.getLogger().Info().
		Int64("NumCommits", consensus.Decider.SignersCount(quorum.Commit)).
		Msg("[finalizeCommits] Finalizing Block")
	beforeCatchupNum := consensus.blockNum
	leaderPriKey, err := consensus.GetConsensusLeaderPrivateKey()
	if err != nil {
		consensus.getLogger().Error().Err(err).Msg("[FinalizeCommits] leader not found")
		return
	}
	// Construct committed message
	network, err := consensus.construct(msg_pb.MessageType_COMMITTED, nil, leaderPriKey)
	if err != nil {
		consensus.getLogger().Warn().Err(err).
			Msg("[FinalizeCommits] Unable to construct Committed message")
		return
	}
	msgToSend, aggSig, FBFTMsg :=
		network.Bytes,
		network.OptionalAggregateSignature,
		network.FBFTMsg
	consensus.aggregatedCommitSig = aggSig // this may not needed
	consensus.FBFTLog.AddMessage(FBFTMsg)
	// find correct block content
	curBlockHash := consensus.blockHash
	block := consensus.FBFTLog.GetBlockByHash(curBlockHash)
	if block == nil {
		consensus.getLogger().Warn().
			Str("blockHash", hex.EncodeToString(curBlockHash[:])).
			Msg("[FinalizeCommits] Cannot find block by hash")
		return
	}

	consensus.tryCatchupDIY()
	if consensus.blockNum-beforeCatchupNum != 1 {
		consensus.getLogger().Warn().
			Uint64("beforeCatchupBlockNum", beforeCatchupNum).
			Msg("[FinalizeCommits] Leader cannot provide the correct block for committed message")
		return
	}

	// if leader success finalize the block, send committed message to validators
	if err := consensus.msgSender.SendWithRetry(
		block.NumberU64(),
		msg_pb.MessageType_COMMITTED, []nodeconfig.GroupID{
			nodeconfig.NewGroupIDByShardID(nodeconfig.ShardID(consensus.ShardID)),
		},
		p2p.ConstructMessage(msgToSend)); err != nil {
		consensus.getLogger().Warn().Err(err).Msg("[finalizeCommits] Cannot send committed message")
	} else {
		consensus.getLogger().Info().
			Hex("blockHash", curBlockHash[:]).
			Uint64("blockNum", consensus.blockNum).
			Msg("[finalizeCommits] Sent Committed Message")
	}

	// Dump new block into level db
	// In current code, we add signatures in block in tryCatchup, the block dump to explorer does not contains signatures
	// but since explorer doesn't need signatures, it should be fine
	// in future, we will move signatures to next block
	//explorer.GetStorageInstance(consensus.leader.IP, consensus.leader.Port, true).Dump(block, beforeCatchupNum)

	if consensus.consensusTimeout[timeoutBootstrap].IsActive() {
		consensus.consensusTimeout[timeoutBootstrap].Stop()
		consensus.getLogger().Info().Msg("[finalizeCommits] Start consensus timer; stop bootstrap timer only once")
	} else {
		consensus.getLogger().Info().Msg("[finalizeCommits] Start consensus timer")
	}
	consensus.consensusTimeout[timeoutConsensus].Start()

	consensus.getLogger().Info().
		Uint64("blockNum", block.NumberU64()).
		Uint64("epochNum", block.Epoch().Uint64()).
		Uint64("ViewId", block.Header().ViewID().Uint64()).
		Str("blockHash", block.Hash().String()).
		Int("numTxns", len(block.Transactions())).
		Int("numStakingTxns", len(block.StakingTransactions())).
		Msg("HOORAY!!!!!!! CONSENSUS REACHED!!!!!!!")

	// Sleep to wait for the full block time
	consensus.getLogger().Info().Msg("[finalizeCommits] Waiting for Block Time")
	<-time.After(time.Until(consensus.NextBlockDue))

	// Send signal to Node to propose the new block for consensus
	// consensus.FinishSignal <- struct{}{}
	consensus.ReadySignal <- struct{}{}

	// Update time due for next block
	consensus.NextBlockDue = time.Now().Add(consensus.BlockPeriod)
}
