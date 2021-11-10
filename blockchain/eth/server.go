package eth

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/version"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"os"
	"path"
)

// Server wraps the Ethereum p2p server, for use with the BDN
type Server struct {
	*p2p.Server
	cancel context.CancelFunc
}

// NewServer return an Ethereum p2p server, configured with BDN friendly defaults
func NewServer(parent context.Context, config *network.EthConfig, bridge blockchain.Bridge, dataDir string, logger log.Logger, ethWSUri string) (*Server, error) {
	var privateKey *ecdsa.PrivateKey

	if config.PrivateKey != nil {
		privateKey = config.PrivateKey
	} else {
		privateKeyPath := path.Join(dataDir, ".gatewaykey")
		privateKeyFromFile, generated, err := LoadOrGeneratePrivateKey(privateKeyPath)

		if err != nil {
			keyWriteErr, ok := err.(keyWriteError)
			if ok {
				logger.Warn("could not write private key", "err", keyWriteErr)
			} else {
				return nil, err
			}
		}

		if generated {
			logger.Warn("no private key found, generating new one", "path", privateKeyPath)
		}
		privateKey = privateKeyFromFile
	}

	ctx, cancel := context.WithCancel(parent)
	backend := NewHandler(ctx, bridge, config, ethWSUri)

	server := p2p.Server{
		Config: p2p.Config{
			PrivateKey:       privateKey,
			MaxPeers:         1,
			MaxPendingPeers:  1,
			DialRatio:        1,
			NoDiscovery:      true,
			DiscoveryV5:      false,
			Name:             fmt.Sprintf("bloXroute Gateway Go v%v", version.BuildVersion),
			BootstrapNodes:   nil,
			BootstrapNodesV5: nil,
			StaticNodes:      config.StaticPeers,
			TrustedNodes:     nil,
			NetRestrict:      nil,
			NodeDatabase:     "",
			Protocols:        MakeProtocols(ctx, backend),
			ListenAddr:       "",
			NAT:              nil,
			Dialer:           nil,
			NoDial:           false,
			EnableMsgEvents:  false,
			Logger:           logger,
		},
		DiscV5: nil,
	}

	s := &Server{
		Server: &server,
		cancel: cancel,
	}
	return s, nil
}

// NewServerWithEthLogger returns the p2p server preconfigured with the default Ethereum logger
func NewServerWithEthLogger(ctx context.Context, config *network.EthConfig, bridge blockchain.Bridge, dataDir string, ethWSUri string) (*Server, error) {
	l := log.New()
	l.SetHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(true)))

	return NewServer(ctx, config, bridge, dataDir, l, ethWSUri)
}

// Stop shutdowns the p2p server and any additional context relevant goroutines
func (s *Server) Stop() {
	s.cancel()
	s.Server.Stop()
}
