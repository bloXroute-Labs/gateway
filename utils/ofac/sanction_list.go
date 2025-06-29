package ofac

import (
	"strings"

	"github.com/bloXroute-Labs/gateway/v2/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// sanctionList is a map of OFAC sanctioned addresses when the primary endpoints are unavailable.
// It must remain read-only and should never be modified
var sanctionList = map[string]bool{
	"0x8576acc5c05d6ce88f4e49bf65bdf0c62f91353c": true,
	"0x901bb9583b24d97e995513c6778dc6888ab6870e": true,
	"0xa7e5d5a720f06526557c513402f2e6b5fa20b008": true,
	"0xd882cfc20f52f2599d84b8e8d58c7fb62cfe344b": true,
	"0x7f367cc41522ce07553e823bf3be79a889debe1b": true,
	"0x1da5821544e25c636c1417ba96ade4cf6d2f9b5a": true,
	"0x7db418b5d567a4e0e8c59ad71be1fce48f3e6107": true,
	"0x72a5843cc08275c8171e582972aa4fda8c397b2a": true,
	"0x7f19720a857f834887fc9a7bc0a0fbe7fc7f8102": true,
	"0x9f4cda013e354b8fc285bf4b9a60460cee7f7ea9": true,
	"0x2f389ce8bd8ff92de3402ffce4691d17fc4f6535": true,
	"0x19aa5fe80d33a56d56c78e82ea5e50e5d80b4dff": true,
	"0xe7aa314c77f4233c18c6cc84384a9247c0cf367b": true,
	"0x308ed4b7b49797e1a98d3818bff6fe5385410370": true,
	"0xfec8a60023265364d066a1212fde3930f6ae8da7": true,
	"0x67d40EE1A85bf4a4Bb7Ffae16De985e8427B6b45": true,
	"0x6f1ca141a28907f78ebaa64fb83a9088b02a8352": true,
	"0x6acdfba02d390b97ac2b2d42a63e85293bcc160e": true,
	"0x48549a34ae37b12f6a30566245176994e17c6b4a": true,
	"0x5512d943ed1f7c8a43f3435c85f7ab68b30121b0": true,
	"0xc455f7fd3e0e12afd51fba5c106909934d8a0e4a": true,
	"0x3cbded43efdaf0fc77b9c55f6fc9988fcc9b757d": true,
	"0x7ff9cfad3877f21d41da833e2f775db0569ee3d9": true,
	"0x098b716b8aaf21512996dc57eb0615e2383e2f96": true,
	"0xa0e1c89ef1a489c9c7de96311ed5ce5d32c20e4b": true,
	"0x3cffd56b47b7b41c56258d9c7731abadc360e073": true,
	"0x53b6936513e738f44fb50d2b9476730c0ab3bfc1": true,
	"0x35fb6f6db4fb05e6a4ce86f2c93691425626d4b1": true,
	"0xf7b31119c2682c88d88d455dbb9d5932c65cf1be": true,
	"0x3e37627deaa754090fbfbb8bd226c1ce66d255e9": true,
	"0x08723392ed15743cc38513c4925f5e6be5c17243": true,
	"0x8589427373d6d84e98730d7795d8f6f8731fda16": true,
	"0x722122df12d4e14e13ac3b6895a86e84145b6967": true,
	"0xdd4c48c0b24039969fc16d1cdf626eab821d3384": true,
	"0xd90e2f925da726b50c4ed8d0fb90ad053324f31b": true,
	"0xd96f2b1c14db8458374d9aca76e26c3d18364307": true,
	"0x4736dcf1b7a3d580672cce6e7c65cd5cc9cfba9d": true,
	"0xd4b88df4d29f5cedd6857912842cff3b20c8cfa3": true,
	"0x910cbd523d972eb0a6f4cae4618ad62622b39dbf": true,
	"0xa160cdab225685da1d56aa342ad8841c3b53f291": true,
	"0xfd8610d20aa15b7b2e3be39b396a1bc3516c7144": true,
	"0xf60dd140cff0706bae9cd734ac3ae76ad9ebc32a": true,
	"0x22aaa7720ddd5388a3c0a3333430953c68f1849b": true,
	"0xba214c1c1928a32bffe790263e38b4af9bfcd659": true,
	"0xb1c8094b234dce6e03f10a5b673c1d8c69739a00": true,
	"0x527653ea119f3e6a1f5bd18fbf4714081d7b31ce": true,
	"0x58e8dcc13be9780fc42e8723d8ead4cf46943df2": true,
	"0xd691f27f38b395864ea86cfc7253969b409c362d": true,
	"0xaeaac358560e11f52454d997aaff2c5731b6f8a6": true,
	"0x1356c899d8c9467c7f71c195612f8a395abf2f0a": true,
	"0xa60c772958a3ed56c1f15dd055ba37ac8e523a0d": true,
	"0x169ad27a470d064dede56a2d3ff727986b15d52b": true,
	"0x0836222f2b2b24a3f36f98668ed8f0b38d1a872f": true,
	"0xf67721a2d8f736e75a49fdd7fad2e31d8676542a": true,
	"0x9ad122c22b14202b4490edaf288fdb3c7cb3ff5e": true,
	"0x905b63fff465b9ffbf41dea908ceb12478ec7601": true,
	"0x07687e702b410fa43f4cb4af7fa097918ffd2730": true,
	"0x94a1b5cdb22c43faab4abeb5c74999895464ddaf": true,
	"0xb541fc07bc7619fd4062a54d96268525cbc6ffef": true,
	"0x12d66f87a04a9e220743712ce6d9bb1b5616b8fc": true,
	"0x47ce0c6ed5b0ce3d3a51fdb1c52dc66a7c3c2936": true,
	"0x23773e65ed146a459791799d01336db287f25334": true,
	"0xd21be7248e0197ee08e0c20d4a96debdac3d20af": true,
	"0x610b717796ad172b316836ac95a2ffad065ceab4": true,
	"0x178169b423a011fff22b9e3f3abea13414ddd0f1": true,
	"0xbb93e510bbcd0b7beb5a853875f9ec60275cf498": true,
	"0x2717c5e28cf931547b621a5dddb772ab6a35b701": true,
	"0x03893a7c7463ae47d46bc7f091665f1893656003": true,
	"0xca0840578f57fe71599d29375e16783424023357": true,
	"0xc2a3829f459b3edd87791c74cd45402ba0a20be3": true,
	"0x3ad9db589d201a710ed237c829c7860ba86510fc": true,
	"0x3aac1cc67c2ec5db4ea850957b967ba153ad6279": true,
	"0x76d85b4c0fc497eecc38902397ac608000a06607": true,
	"0x0e3a09dda6b20afbb34ac7cd4a6881493f3e7bf7": true,
	"0x723b78e67497e85279cb204544566f4dc5d2aca0": true,
	"0xcc84179ffd19a1627e79f8648d09e095252bc418": true,
	"0x6bf694a291df3fec1f7e69701e3ab6c592435ae7": true,
	"0x330bdfade01ee9bf63c209ee33102dd334618e0a": true,
	"0xa5c2254e4253490c54cef0a4347fddb8f75a4998": true,
	"0xaf4c0b70b2ea9fb7487c7cbb37ada259579fe040": true,
	"0xdf231d99ff8b6c6cbf4e9b9a945cbacef9339178": true,
	"0x1e34a77868e19a6647b1f2f47b51ed72dede95dd": true,
	"0xd47438c816c9e7f2e2888e060936a499af9582b3": true,
	"0x84443cfd09a48af6ef360c6976c5392ac5023a1f": true,
	"0xd5d6f8d9e784d0e26222ad3834500801a68d027d": true,
	"0xaf8d1839c3c67cf571aa74b5c12398d4901147b3": true,
	"0x407cceeaa7c95d2fe2250bf9f2c105aa7aafb512": true,
	"0x05e0b5b40b7b66098c2161a5ee11c5740a3a7c45": true,
	"0xd8d7de3349ccaa0fde6298fe6d7b7d0d34586193": true,
	"0x3efa30704d2b8bbac821307230376556cf8cc39e": true,
	"0x746aebc06d2ae31b71ac51429a19d54e797878e9": true,
	"0x5f6c97c6ad7bdd0ae7e0dd4ca33a4ed3fdabd4d7": true,
	"0xf4b067dd14e95bab89be928c07cb22e3c94e0daa": true,
	"0x01e2919679362dfbc9ee1644ba9c6da6d6245bb1": true,
	"0x2fc93484614a34f26f7970cbb94615ba109bb4bf": true,
	"0x26903a5a198d571422b2b4ea08b56a37cbd68c89": true,
	"0xb20c66c4de72433f3ce747b58b86830c459ca911": true,
	"0x2573bac39ebe2901b4389cd468f2872cf7767faf": true,
	"0x653477c392c16b0765603074f157314cc4f40c32": true,
	"0x88fd245fedec4a936e700f9173454d1931b4c307": true,
	"0x09193888b3f38c82dedfda55259a82c0e7de875e": true,
	"0x5cab7692d4e94096462119ab7bf57319726eed2a": true,
	"0x756c4628e57f7e7f8a459ec2752968360cf4d1aa": true,
	"0xd82ed8786d7c69dc7e052f7a542ab047971e73d2": true,
	"0x77777feddddffc19ff86db637967013e6c6a116c": true,
	"0x833481186f16cece3f1eeea1a694c42034c3a0db": true,
	"0xb04e030140b30c27bcdfaafffa98c57d80eda7b4": true,
	"0xcee71753c9820f063b38fdbe4cfdaf1d3d928a80": true,
	"0x8281aa6795ade17c8973e1aedca380258bc124f9": true,
	"0x57b2b8c82f065de8ef5573f9730fc1449b403c9f": true,
	"0x23173fe8b96a4ad8d2e17fb83ea5dcccdca1ae52": true,
	"0x538ab61e8a9fc1b2f93b3dd9011d662d89be6fe6": true,
	"0x94be88213a387e992dd87de56950a9aef34b9448": true,
	"0x242654336ca2205714071898f67e254eb49acdce": true,
	"0x776198ccf446dfa168347089d7338879273172cf": true,
	"0xedc5d01286f99a066559f60a585406f3878a033e": true,
	"0xd692fd2d0b2fbd2e52cfa5b5b9424bc981c30696": true,
	"0xdf3a408c53e5078af6e8fb2a85088d46ee09a61b": true,
	"0x743494b60097a2230018079c02fe21a7b687eaa5": true,
	"0x94c92f096437ab9958fc0a37f09348f30389ae79": true,
	"0x5efda50f22d34f262c29268506c5fa42cb56a1ce": true,
	"0x2f50508a8a3d323b91336fa3ea6ae50e55f32185": true,
	"0x179f48c78f57a3a78f0608cc9197b8972921d1d2": true,
	"0xffbac21a641dcfe4552920138d90f3638b3c9fba": true,
	"0xd0975b32cea532eadddfc9c60481976e39db3472": true,
	"0x1967d8af5bd86a497fb3dd7899a020e47560daaf": true,
	"0x83e5bc4ffa856bb84bb88581f5dd62a433a25e0d": true,
	"0x08b2eFdcdB8822EfE5ad0Eae55517cf5DC544251": true,
	"0x04DBA1194ee10112fE6C3207C0687DEf0e78baCf": true,
	"0x0Ee5067b06776A89CcC7dC8Ee369984AD7Db5e06": true,
	"0x502371699497d08D5339c870851898D6D72521Dd": true,
	"0x5A14E72060c11313E38738009254a90968F58f51": true,
	"0xEFE301d259F525cA1ba74A7977b80D5b060B3ccA": true,
	"0x39d908dac893cbcb53cc86e0ecc369aa4def1a29": true,
	"0xdcbEfFBECcE100cCE9E4b153C4e15cB885643193": true,
	"0x5f48c2a71b2cc96e3f0ccae4e39318ff0dc375b2": true,
	"0x5a7a51bfb49f190e5a6060a5bc6052ac14a3b59f": true,
	"0xed6e0a7e4ac94d976eebfb82ccf777a3c6bad921": true,
	"0x797d7ae72ebddcdea2a346c1834e04d1f8df102b": true,
	"0x931546D9e66836AbF687d2bc64B30407bAc8C568": true,
	"0x43fa21d92141BA9db43052492E0DeEE5aa5f0A93": true,
	"0x6be0ae71e6c41f2f9d0d1a3b8d0f75e6f6a0b46e": true,
}

// ShouldBlockTransaction checks the sanction list to see if 'from' or 'to' are on block list and returns any blocked addresses for stats
func ShouldBlockTransaction(transaction *ethtypes.Transaction, oFACList *types.OFACMap) ([]string, bool) {
	sender, err := types.LatestSignerForChainID(transaction.ChainId()).Sender(transaction)
	if err != nil {
		return nil, false
	}
	fromAddress := strings.ToLower(sender.Hex())

	toAddress := ""
	if transaction.To() != nil {
		toAddress = strings.ToLower(transaction.To().Hex())
	}

	blockedAddresses := make([]string, 0, 2)

	if isBlocked(fromAddress, oFACList) {
		blockedAddresses = append(blockedAddresses, fromAddress)
	}

	if toAddress != "" && fromAddress != toAddress && isBlocked(toAddress, oFACList) {
		blockedAddresses = append(blockedAddresses, toAddress)
	}

	if len(blockedAddresses) == 0 {
		return nil, false
	}
	return blockedAddresses, true
}

func isBlocked(address string, oFACList *types.OFACMap) bool {
	// try OFAC list first
	if oFACList != nil && oFACList.Size() > 0 {
		if _, found := oFACList.Load(address); found {
			return true
		}
	}
	_, found := sanctionList[address]
	return found
}
