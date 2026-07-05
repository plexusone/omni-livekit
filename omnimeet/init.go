// Package omnimeet provides a LiveKit implementation of the OmniMeet provider interface.
package omnimeet

import (
	omnimeet "github.com/plexusone/omnimeet-core"
)

func init() {
	// Register the LiveKit provider with the OmniMeet registry
	omnimeet.RegisterMeetingProvider(providerName, NewProviderFromConfig, omnimeet.PriorityThick)
}
