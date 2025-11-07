package qlog

type CongestionState string

const (
	// CongestionStateSlowStart is the slow start phase of Reno / Cubic
	CongestionStateSlowStart CongestionState = "slow_start"
	// CongestionStateCongestionAvoidance is the congestion avoidance phase of Reno / Cubic
	CongestionStateCongestionAvoidance CongestionState = "congestion_avoidance"
	// CongestionStateRecovery is the recovery phase of Reno / Cubic
	CongestionStateRecovery CongestionState = "recovery"
	// CongestionStateApplicationLimited means that the congestion controller is application limited
	CongestionStateApplicationLimited CongestionState = "application_limited"
)

func (s CongestionState) String() string {
	return string(s)
}

type CongestionStateUpdated struct {
	State CongestionState
}
