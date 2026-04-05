package ws

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func BenchmarkBuildNotificationContent(b *testing.B) {
	block := bxmock.NewEthBlock(10, common.Hash{})
	notif, err := types.NewEthBlockNotification("Mainnet", block.Hash(), block, nil)
	if err != nil {
		b.Fatal(err)
	}
	notif.SetNotificationType(types.NewBlocksFeed)

	h := &handlerObj{senderExtractor: services.NewSenderExtractor()}
	includes := []string{"hash", "header", "transactions", "future_validator_info"}

	// ParsedTxs=true: common case, includes prep is zero-alloc (no copy)
	b.Run("ParsedTxs=true", func(b *testing.B) {
		clientReq := &ClientReq{Includes: includes, ParsedTxs: true}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			incl := clientReq.Includes
			h.buildNotificationContent(notif, incl)
		}
	})

	// ParsedTxs=false with "transactions": copy path triggered
	b.Run("ParsedTxs=false/with_transactions", func(b *testing.B) {
		clientReq := &ClientReq{Includes: includes, ParsedTxs: false}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			incl := clientReq.Includes
			if !clientReq.ParsedTxs {
				for _, inc := range incl {
					if inc == "transactions" {
						cp := make([]string, len(clientReq.Includes))
						copy(cp, clientReq.Includes)
						for j, s := range cp {
							if s == "transactions" {
								cp[j] = "raw_transactions"
							}
						}
						incl = cp
						break
					}
				}
			}
			h.buildNotificationContent(notif, incl)
		}
	})

	// ParsedTxs=false without "transactions": no copy needed (lazy path, zero-alloc prep)
	b.Run("ParsedTxs=false/no_transactions", func(b *testing.B) {
		clientReq := &ClientReq{
			Includes:  []string{"hash", "header", "future_validator_info"},
			ParsedTxs: false,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			incl := clientReq.Includes
			if !clientReq.ParsedTxs {
				for _, inc := range incl {
					if inc == "transactions" {
						cp := make([]string, len(clientReq.Includes))
						copy(cp, clientReq.Includes)
						for j, s := range cp {
							if s == "transactions" {
								cp[j] = "raw_transactions"
							}
						}
						incl = cp
						break
					}
				}
			}
			h.buildNotificationContent(notif, incl)
		}
	})
}
