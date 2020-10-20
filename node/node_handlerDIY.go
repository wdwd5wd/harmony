package node

import (
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/internal/utils"
)

// PreannounceDIY 把一些需要信息先打包
func (node *Node) PreannounceDIY(newBlock *types.Block) {
	if _, err := node.Blockchain().InsertChain([]*types.Block{newBlock}, true); err != nil {
		utils.Logger().Error().
			Err(err).
			Uint64("blockNum", newBlock.NumberU64()).
			Str("parentHash", newBlock.Header().ParentHash().Hex()).
			Str("hash", newBlock.Header().Hash().Hex()).
			Msg("Error Adding new block to blockchain")
		return
	}
	utils.Logger().Info().
		Uint64("blockNum", newBlock.NumberU64()).
		Str("hash", newBlock.Header().Hash().Hex()).
		Msg("Added New Block to Blockchain!!!")

}
