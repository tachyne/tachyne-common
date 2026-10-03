package gwsession

// reconfigure.go is the gateway's half of vanilla reconfiguration
// (ServerGamePacketListenerImpl.switchToConfig → handleConfiguration-
// Acknowledged → a new ServerConfigurationPacketListenerImpl → its
// handleConfigurationFinished → PlayerList.placeNewPlayer). The world asks
// for it (attach MsgStartConfiguration); the gateway sends
// start_configuration and from then on sends no play packet, runs the
// configuration phase on the client reader once the client acknowledges,
// tells the world when the client finishes (MsgConfigured), and resumes play
// with the login packet when the world places the player again (MsgRejoin).

import (
	"errors"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 ids of the reconfiguration packets (GameProtocols order).
const (
	playClientStartConfig = 0x6f // start_configuration (empty body)
	playClientUpdateTags  = 0x7f // update_tags (play)
	playServerConfigAck   = 0x0e // configuration_acknowledged (empty body)
)

// Connection phases (clientConn.phase).
const (
	phasePlay        = iota // in play: packets flow
	phaseConfiguring        // sent back to configuration, or in it
	phaseRejoining          // configured again; waiting for the world's rejoin
)

var (
	errUnrequestedConfig = errors.New("client acknowledged configuration, but none was requested")
	errEarlyRejoin       = errors.New("world rejoined the player before configuration finished")
)

// startConfiguration is switchToConfig: start_configuration, then nothing in
// play. A second request while one is under way is ignored.
func (cc *clientConn) startConfiguration(e attach.StartConfiguration) error {
	id, data, drop := cc.tr.Clientbound(protocol.StatePlay, playClientStartConfig, nil)
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.phase != phasePlay {
		return nil
	}
	cc.phase = phaseConfiguring
	cc.pendingConfig = &e
	if drop {
		return nil
	}
	return protocol.WriteCompressed(cc.c, id, data, compressThreshold)
}

// takeConfig is handleConfigurationAcknowledged's check: the configuration
// the world asked for, or nil when none was (vanilla throws, which
// disconnects the client).
func (cc *clientConn) takeConfig() *attach.StartConfiguration {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.phase != phaseConfiguring {
		return nil
	}
	p := cc.pendingConfig
	cc.pendingConfig = nil
	return p
}

// configured marks the client done configuring: it is in play again, but
// waits for the login packet.
func (cc *clientConn) configured() {
	cc.mu.Lock()
	cc.phase = phaseRejoining
	cc.mu.Unlock()
}

// configuring reports whether the client is between phases (the keep-alive
// skips play keep-alives then).
func (cc *clientConn) configuring() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.phase != phasePlay
}

// rejoin ends the reconfiguration with the login packet, written under the
// same lock that reopens play, so no other play packet can precede it.
func (cc *clientConn) rejoin(login []byte) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.phase != phaseRejoining {
		return errEarlyRejoin
	}
	id, data, drop := cc.tr.Clientbound(protocol.StatePlay, playClientLogin, login)
	cc.phase = phasePlay
	if drop {
		return nil
	}
	return protocol.WriteCompressed(cc.c, id, data, compressThreshold)
}
