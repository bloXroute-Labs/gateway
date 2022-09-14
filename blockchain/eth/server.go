package eth

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"os"
	"path"
)

// Server wraps the Ethereum p2p server, for use with the BDN
type Server struct {
	p2pServer *p2p.Server
	cancel    context.CancelFunc
}

// NewServer return an Ethereum p2p server, configured with BDN friendly defaults
func NewServer(parent context.Context, config *network.EthConfig, bridge blockchain.Bridge, dataDir string, logger log.Logger, ws blockchain.WSManager) (*Server, error) {
	var privateKey *ecdsa.PrivateKey

	if config.PrivateKey != nil {
		privateKey = config.PrivateKey
	} else {
		privateKeyPath := path.Join(dataDir, ".gatewaykey")
		privateKeyFromFile, generated, err := network.LoadOrGeneratePrivateKey(privateKeyPath)

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
	backend := NewHandler(ctx, bridge, config, ws)

	server := p2p.Server{
		Config: p2p.Config{
			PrivateKey:       privateKey,
			MaxPeers:         len(config.StaticEnodes()),
			MaxPendingPeers:  1,
			DialRatio:        1,
			NoDiscovery:      true,
			DiscoveryV5:      false,
			Name:             fmt.Sprintf("bloXroute Gateway Go v%v", version.BuildVersion),
			BootstrapNodes:   nil,
			BootstrapNodesV5: nil,
			StaticNodes:      config.StaticEnodes(),
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
		p2pServer: &server,
		cancel:    cancel,
	}
	return s, nil
}

// NewServerWithEthLogger returns the p2p server preconfigured with the default Ethereum logger
func NewServerWithEthLogger(ctx context.Context, config *network.EthConfig, bridge blockchain.Bridge, dataDir string, ws blockchain.WSManager) (*Server, error) {
	l := log.New()
	l.SetHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(true)))

	return NewServer(ctx, config, bridge, dataDir, l, ws)
}

// Start starts eth server
func (s *Server) Start() error {
	if err := s.p2pServer.Start(); err != nil {
		return err
	}

	return nil
}

// Stop shutdowns the p2p server and any additional context relevant goroutines
func (s *Server) Stop() {
	s.cancel()
	s.p2pServer.Stop()
}

// AddEthLoggerFileHandler registers additional file handler by file path
func (s *Server) AddEthLoggerFileHandler(path string) error {
	fileHandler, err := log.FileHandler(path, log.TerminalFormat(false))
	if err != nil {
		return err
	}
	s.p2pServer.Logger.SetHandler(log.MultiHandler(fileHandler, s.p2pServer.Logger.GetHandler()))
	return nil
}
