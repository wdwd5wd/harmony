package consensus

// switchPhase will switch FBFTPhase to nextPhase if the desirePhase equals the nextPhase
func (consensus *Consensus) switchPhaseDIY(desired FBFTPhase, override bool) {
	if override {
		consensus.phase = desired
		return
	}

	var nextPhase FBFTPhase
	switch consensus.phase {
	case FBFTAnnounce:
		nextPhase = FBFTCommit

	case FBFTCommit:
		nextPhase = FBFTAnnounce
	}
	if nextPhase == desired {
		consensus.phase = nextPhase
	}
}
