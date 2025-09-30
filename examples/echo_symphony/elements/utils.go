package elements

import (
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

var (
	elementTable map[string]func() element.RPCElement = map[string]func() element.RPCElement{
		"metrics":          NewMetricsElement,
		"firewall":         NewFirewallElement,
		"cacheweak":        NewCacheweakElement,
		"admissioncontrol": NewAdmissioncontrolElement,
		"bandwidthlimit":   NewBandwidthlimitElement,
	}
)

func GetElementTable() map[string]func() element.RPCElement {
	return elementTable
}
