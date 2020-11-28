package blockproposal

// NewDIY returns a block proposal service.
func NewDIY(readySignal chan struct{}, finishSignal chan struct{}, waitForConsensusReadyDIY func(readySignal chan struct{}, finishSignal chan struct{}, stopChan chan struct{}, stoppedChan chan struct{})) *Service {
	return &Service{readySignal: readySignal, finishSignal: finishSignal, waitForConsensusReadyDIY: waitForConsensusReadyDIY}
}
