package chain

import (
	"github.com/harmony-one/harmony/block"
	"github.com/harmony-one/harmony/consensus/engine"
	"github.com/harmony-one/harmony/core/state"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/shard"
	"github.com/harmony-one/harmony/staking/availability"
	"github.com/harmony-one/harmony/staking/slash"
	staking "github.com/harmony-one/harmony/staking/types"
	"github.com/pkg/errors"
)

// Finalize implements Engine, accumulating the block rewards,
// setting the final state and assembling the block.
func (e *engineImpl) FinalizeDIY(
	chain engine.ChainReader, header *block.Header,
	state *state.DB, txs []*types.Transaction,
	receipts []*types.Receipt, outcxs []*types.CXReceipt,
	incxs []*types.CXReceiptsProof, stks staking.StakingTransactions,
	doubleSigners slash.Records,
) (*types.Block, error) {

	isBeaconChain := header.ShardID() == shard.BeaconChainShardID
	isNewEpoch := len(header.ShardState()) > 0
	inPreStakingEra := chain.Config().IsPreStaking(header.Epoch())
	inStakingEra := chain.Config().IsStaking(header.Epoch())

	// Process Undelegations, set LastEpochInCommittee and set EPoS status
	// Needs to be before AccumulateRewardsAndCountSigs
	if isBeaconChain && isNewEpoch && inPreStakingEra {
		if err := payoutUndelegations(chain, header, state); err != nil {
			return nil, err
		}

		// Needs to be after payoutUndelegations because payoutUndelegations
		// depends on the old LastEpochInCommittee
		if err := setLastEpochInCommittee(header, state); err != nil {
			return nil, err
		}

		curShardState, err := chain.ReadShardState(chain.CurrentBlock().Epoch())
		if err != nil {
			return nil, err
		}
		// Needs to be before AccumulateRewardsAndCountSigs because
		// ComputeAndMutateEPOSStatus depends on the signing counts that's
		// consistent with the counts when the new shardState was proposed.
		// Refer to committee.IsEligibleForEPoSAuction()
		for _, addr := range curShardState.StakedValidators().Addrs {
			if err := availability.ComputeAndMutateEPOSStatus(
				chain, state, addr,
			); err != nil {
				return nil, err
			}
		}
	}

	// // Accumulate block rewards and commit the final state root
	// // Header seems complete, assemble into a block and return
	// payout, err := AccumulateRewardsAndCountSigs(
	// 	chain, state, header, e.Beaconchain(),
	// )
	// if err != nil {
	// 	return nil, errors.New("cannot pay block reward")
	// }

	// Apply slashes
	if isBeaconChain && inStakingEra && len(doubleSigners) > 0 {
		if err := applySlashes(chain, header, state, doubleSigners); err != nil {
			return nil, err
		}
	} else if len(doubleSigners) > 0 {
		return nil, errors.New("slashes proposed in non-beacon chain or non-staking epoch")
	}

	// Finalize the state root
	header.SetRoot(state.IntermediateRoot(chain.Config().IsS3(header.Epoch())))
	return types.NewBlock(header, txs, receipts, outcxs, incxs, stks), nil
}
