package bxmessage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
)

func TestMEVBundle(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:      time.Now().UTC(),
		OriginalSenderAccountID:   "bloXroute LABS",
		OriginalSenderAccountTier: sdnmessage.ATierUltra,
		SentFromCloudAPI:          true,
		AvoidMixedBundles:         true,
		PriorityFeeRefund:         true,
		BlocksCount:               2,
		DroppingTxHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		EndOfBlock: true,
	}

	b, err := m.Pack(CurrentProtocol)
	require.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, CurrentProtocol)
	require.NoError(t, err)

	m.msgType = MEVBundleType
	require.Equal(t, m, m2)
}

func TestMEVBundle_EmptyBuilders(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders:               map[string]string{},
		PerformanceTimestamp:      time.Now().UTC(),
		OriginalSenderAccountID:   "bloXroute LABS",
		OriginalSenderAccountTier: sdnmessage.ATierUltra,
		SentFromCloudAPI:          true,
		DroppingTxHashes:          []string{},
		EndOfBlock:                false,
	}

	b, err := m.Pack(BundlesOverBDNOriginalSenderTierProtocol)
	require.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundlesOverBDNOriginalSenderTierProtocol)
	require.NoError(t, err)

	m.msgType = MEVBundleType
	require.Equal(t, m, m2)
}

func TestMEVBundle_NoOriginalSenderAccountTier(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		SentFromCloudAPI:        true,
		DroppingTxHashes:        []string{},
		EndOfBlock:              false,
	}

	b, err := m.Pack(BundlesOverBDNOriginalSenderTierProtocol)
	require.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundlesOverBDNOriginalSenderTierProtocol)
	require.NoError(t, err)

	m.msgType = MEVBundleType
	require.Equal(t, m, m2)
	assert.Equal(t, sdnmessage.AccountTier(""), m2.OriginalSenderAccountTier)
}

func TestMEVBundlePayoutBackCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp: time.Now(),
		DroppingTxHashes:     []string{},
		EndOfBlock:           false,
	}

	b, err := m.Pack(BundlesOverBDNPayoutProtocol - 1)
	assert.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundlesOverBDNPayoutProtocol-1)
	assert.NoError(t, err)

	m.msgType = MEVBundleType

	// There is precision loss due to using float64 for timestamp
	assert.Less(t, m.PerformanceTimestamp.Sub(m2.PerformanceTimestamp), time.Millisecond)
	m2.PerformanceTimestamp = m.PerformanceTimestamp

	assert.Equal(t, m, m2)
}

func TestMEVBundleOriginalSenderAccountBackCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		DroppingTxHashes:        []string{},
		EndOfBlock:              false,
	}

	b, err := m.Pack(BundlesOverBDNOriginalSenderAccountProtocol - 1)
	assert.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundlesOverBDNOriginalSenderAccountProtocol-1)
	assert.NoError(t, err)

	m.msgType = MEVBundleType

	// New fields
	m.OriginalSenderAccountID = ""

	assert.Equal(t, m, m2)
}

func TestMEVBundleAvoidMixedBundleBackCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		AvoidMixedBundles:       true,
		DroppingTxHashes:        []string{},
		EndOfBlock:              false,
	}

	b, err := m.Pack(AvoidMixedBundleProtocol - 1)
	assert.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, AvoidMixedBundleProtocol-1)
	assert.NoError(t, err)

	m.msgType = MEVBundleType

	// new field
	m.AvoidMixedBundles = false

	assert.Equal(t, m, m2)
}

func TestMevBundlePriorityFeeRefundBackCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		PriorityFeeRefund:       true,
		DroppingTxHashes:        []string{},
		EndOfBlock:              false,
	}

	t.Run("BundlePriorityFeeRefundProtocol - 1 protocol", func(t *testing.T) {
		b, err := m.Pack(BundlePriorityFeeRefundProtocol - 1)
		assert.NoError(t, err)

		var m2 MEVBundle
		err = m2.Unpack(b, BundlePriorityFeeRefundProtocol-1)
		assert.NoError(t, err)

		m.msgType = MEVBundleType

		// new field
		m.PriorityFeeRefund = false

		assert.Equal(t, m, m2)
	})

	t.Run("current protocol", func(t *testing.T) {
		b, err := m.Pack(CurrentProtocol)
		assert.NoError(t, err)

		var m2 MEVBundle
		err = m2.Unpack(b, CurrentProtocol)
		assert.NoError(t, err)

		m.msgType = MEVBundleType

		assert.Equal(t, m, m2)
	})
}

func TestMEVBundleBlocksCountAndDroppingTxsBackCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		AvoidMixedBundles:       true,
		BlocksCount:             2,
		DroppingTxHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
		},
		EndOfBlock: false,
	}

	b, err := m.Pack(BundleBlocksCountAndDroppingTxs - 1)
	assert.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundleBlocksCountAndDroppingTxs-1)
	assert.NoError(t, err)

	m.msgType = MEVBundleType

	// new fields
	m.BlocksCount = 0
	m.DroppingTxHashes = []string{}

	assert.Equal(t, m, m2)
}

func TestMEVBundleEndOfBlockCompatibility(t *testing.T) {
	m := MEVBundle{
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp:    time.Now().UTC(),
		OriginalSenderAccountID: "bloXroute LABS",
		AvoidMixedBundles:       true,
		BlocksCount:             2,
		DroppingTxHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
		},
		EndOfBlock: false,
	}

	b, err := m.Pack(BundlesEndOfBlocks - 1)
	assert.NoError(t, err)

	var m2 MEVBundle
	err = m2.Unpack(b, BundlesEndOfBlocks-1)
	assert.NoError(t, err)

	m.msgType = MEVBundleType

	// new fields
	m.EndOfBlock = false

	assert.Equal(t, m, m2)
}
