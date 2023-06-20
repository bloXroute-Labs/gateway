package bxmessage

import (
	"encoding/hex"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/utils"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPackMEVBundlePackSuccess(t *testing.T) {
	mockClock := utils.MockClock{}
	clock = &mockClock

	// case 1, normal situation
	// test Tx that got packed at certain time, and unpacked after one second
	// 2022-03-22 11:35:33.797682 -0500 CDT m=+298.247378704
	// 1011011011110110000+01001001111101100111011101101000+0010110000
	mockClock.SetTime(time.Unix(0, 1647966890567246000)) // 2022-03-22 11:35:33.79
	packTime := mockClock.Now()
	fmt.Printf("packTime in pack: %v\n", packTime)

	m := MEVBundle{
		ID:         "1",
		JSONRPC:    "2.0",
		Method:     "eth_sendBundle",
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
		Frontrunning: true,
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		PerformanceTimestamp: packTime,
	}

	_, err := m.Pack(BundlesOverBDNProtocol)

	if err != nil {
		t.Fatalf("Failed to pack MEVBundle: %s", err)
	}
}

func TestUnpackMEVBundle(t *testing.T) {
	mockClock := utils.MockClock{}
	clock = &mockClock

	// case 1, normal situation
	// test Tx that got packed at certain time, and unpacked after one second
	// 2022-03-22 11:35:33.797682 -0500 CDT m=+298.247378704
	// 1011011011110110000+01001001111101100111011101101000+0010110000
	mockClock.SetTime(time.Unix(0, 1647966890567246000)) // 2022-03-22 11:35:33.79
	packTime := mockClock.Now()

	buffer := "fffefdfc6d657662756e646c6500000086030000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000e006574685f73656e6442756e646c65123e4567e89b12d3a45642661417400003005f00f85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb5f00f85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a0275f00f85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f0800000000000000002a5661807b57610200a74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f01432d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d010208006275696c646572310500307831323308006275696c6465723205003078343536c34da4aa7e8ed84100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001"
	packedData, err := hex.DecodeString(buffer)
	if err != nil {
		t.Fatalf("Failed to decode hex string: %s", err)
	}

	var m MEVBundle
	err = m.Unpack(packedData, BundlesOverBDNProtocol)
	if err != nil {
		t.Fatalf("Failed to unpack MEVBundle: %s", err)
	}

	expectedMEVBundle := MEVBundle{
		ID:      "1",
		JSONRPC: "2.0",
		Method:  "eth_sendBundle",
		UUID:    "123e4567-e89b-12d3-a456-426614174000",
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
		Frontrunning: true,
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
		BundleHash:           "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		PerformanceTimestamp: packTime,
	}

	assert.Equal(t, expectedMEVBundle.Method, m.Method)
	assert.Equal(t, expectedMEVBundle.UUID, m.UUID)
	assert.Equal(t, expectedMEVBundle.Transactions[0], m.Transactions[0])
	assert.Equal(t, expectedMEVBundle.Transactions[1], m.Transactions[1])
	assert.Equal(t, expectedMEVBundle.Transactions[2], m.Transactions[2])
	assert.Equal(t, expectedMEVBundle.BlockNumber, m.BlockNumber)
	assert.Equal(t, expectedMEVBundle.MinTimestamp, m.MinTimestamp)
	assert.Equal(t, expectedMEVBundle.MaxTimestamp, m.MaxTimestamp)
	assert.Equal(t, expectedMEVBundle.RevertingHashes[0], m.RevertingHashes[0])
	assert.Equal(t, expectedMEVBundle.RevertingHashes[1], m.RevertingHashes[1])
	assert.Equal(t, expectedMEVBundle.Frontrunning, m.Frontrunning)
	assert.Equal(t, expectedMEVBundle.BundleHash, m.BundleHash)
	assert.True(t, test.MapsEqual(expectedMEVBundle.MEVBuilders, m.MEVBuilders))
}
