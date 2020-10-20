package core

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/harmony-one/harmony/core/state"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/core/vm"
	"github.com/harmony-one/harmony/staking/slash"
	"github.com/pkg/errors"
)

// ProcessDIY processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) ProcessDIY(
	block *types.Block, statedb *state.DB, cfg vm.Config,
) (
	types.Receipts, types.CXReceipts,
	[]*types.Log, uint64, error,
) {
	var (
		receipts types.Receipts
		outcxs   types.CXReceipts
		incxs    = block.IncomingReceipts()
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
	)

	beneficiary, err := p.bc.GetECDSAFromCoinbase(header)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, cxReceipt, _, err := ApplyTransaction(
			p.config, p.bc, &beneficiary, gp, statedb, header, tx, usedGas, cfg,
		)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		if cxReceipt != nil {
			outcxs = append(outcxs, cxReceipt)
		}
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Iterate over and process the staking transactions
	L := len(block.Transactions())
	for i, tx := range block.StakingTransactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i+L)
		receipt, _, err := ApplyStakingTransaction(
			p.config, p.bc, &beneficiary, gp, statedb, header, tx, usedGas, cfg,
		)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}

	// incomingReceipts should always be processed
	// after transactions (to be consistent with the block proposal)
	for _, cx := range block.IncomingReceipts() {
		if err := ApplyIncomingReceipt(
			p.config, statedb, header, cx,
		); err != nil {
			return nil, nil,
				nil, 0, errors.New("[Process] Cannot apply incoming receipts")
		}
	}

	slashes := slash.Records{}
	if s := header.Slashes(); len(s) > 0 {
		if err := rlp.DecodeBytes(s, &slashes); err != nil {
			return nil, nil, nil, 0, errors.New(
				"[Process] Cannot finalize block",
			)
		}
	}

	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	_, err = p.engine.FinalizeDIY(
		p.bc, header, statedb, block.Transactions(),
		receipts, outcxs, incxs, block.StakingTransactions(), slashes,
	)
	if err != nil {
		return nil, nil, nil, 0, errors.New("[Process] Cannot finalize block")
	}

	return receipts, outcxs, allLogs, *usedGas, nil
}
