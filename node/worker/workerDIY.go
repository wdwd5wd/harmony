package worker

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/harmony-one/harmony/core"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/internal/utils"
	"github.com/harmony-one/harmony/shard"
	"github.com/pkg/errors"
)

// UpdateCurrentDIY updates the current environment with the current state and header.
func (w *Worker) UpdateCurrentDIY() error {
	parent := w.chain.CurrentBlock()
	num := parent.Number()
	timestamp := time.Now().Unix()

	epoch := w.GetNewEpoch()
	header := w.factory.NewHeader(epoch).With().
		// ParentHash(parent.Hash()).
		Number(num.Add(num, common.Big2)).
		GasLimit(core.CalcGasLimit(parent, w.gasFloor, w.gasCeil)).
		Time(big.NewInt(timestamp)).
		ShardID(w.chain.ShardID()).
		Header()
	return w.makeCurrent(parent, header)
}

// FinalizeNewBlockDIY generate a new block for the next consensus round.
func (w *Worker) FinalizeNewBlockDIY(
	sig []byte, signers []byte, viewID uint64, coinbase common.Address,
	crossLinks types.CrossLinks, shardState *shard.State,
) (*types.Block, error) {
	// Put sig, signers, viewID, coinbase into header
	if len(sig) > 0 && len(signers) > 0 {
		// TODO: directly set signature into lastCommitSignature
		sig2 := w.current.header.LastCommitSignature()
		copy(sig2[:], sig[:])
		w.current.header.SetLastCommitSignature(sig2)
		w.current.header.SetLastCommitBitmap(signers)
	}

	w.current.header.SetCoinbase(coinbase)
	w.current.header.SetViewID(new(big.Int).SetUint64(viewID))

	// Put crosslinks into header
	if len(crossLinks) > 0 {
		crossLinks.Sort()
		crossLinkData, err := rlp.EncodeToBytes(crossLinks)
		if err == nil {
			utils.Logger().Debug().
				Uint64("blockNum", w.current.header.Number().Uint64()).
				Int("numCrossLinks", len(crossLinks)).
				Msg("Successfully proposed cross links into new block")
			w.current.header.SetCrossLinks(crossLinkData)
		} else {
			utils.Logger().Debug().Err(err).Msg("Failed to encode proposed cross links")
			return nil, err
		}
	} else {
		utils.Logger().Debug().Msg("Zero crosslinks to finalize")
	}

	// Put slashes into header
	if w.config.IsStaking(w.current.header.Epoch()) {
		doubleSigners := w.current.slashes
		if len(doubleSigners) > 0 {
			if data, err := rlp.EncodeToBytes(doubleSigners); err == nil {
				w.current.header.SetSlashes(data)
				utils.Logger().Info().
					Msg("encoded slashes into headers of proposed new block")
			} else {
				utils.Logger().Debug().Err(err).Msg("Failed to encode proposed slashes")
				return nil, err
			}
		}
	}

	// Put shard state into header
	if shardState != nil && len(shardState.Shards) != 0 {
		//we store shardstatehash in header only before prestaking epoch (header v0,v1,v2)
		if !w.config.IsPreStaking(w.current.header.Epoch()) {
			w.current.header.SetShardStateHash(shardState.Hash())
		}
		isStaking := false
		if shardState.Epoch != nil && w.config.IsStaking(shardState.Epoch) {
			isStaking = true
		}
		// NOTE: Besides genesis, this is the only place where the shard state is encoded.
		shardStateData, err := shard.EncodeWrapper(*shardState, isStaking)
		if err == nil {
			w.current.header.SetShardState(shardStateData)
		} else {
			utils.Logger().Debug().Err(err).Msg("Failed to encode proposed shard state")
			return nil, err
		}
	}
	state := w.current.state.Copy()
	copyHeader := types.CopyHeader(w.current.header)
	block, err := w.engine.FinalizeDIY(
		w.chain, copyHeader, state, w.current.txs, w.current.receipts,
		w.current.outcxs, w.current.incxs, w.current.stakingTxs,
		w.current.slashes,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot finalize block")
	}

	return block, nil
}
