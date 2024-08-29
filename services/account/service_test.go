package account

/*
func TestGateway_Authorize(t *testing.T) {
	type fields struct {
		accountFetcher  func(accountID types.AccountID) (*accountResult, error)
		sdnAccountModel sdnmessage.Account
	}
	type args struct {
		accountID                    types.AccountID
		secretHash                   string
		allowAccessToInternalGateway bool
		allowIntroductoryTierAccess  bool
	}
	sdnAccountModelEnterprise := sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: testGatewayAccountID,
			TierName:  sdnmessage.ATierEnterprise,
		},
		SecretHash: testGatewaySecretHash,
	}
	sdnAccountModelProfessional := sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: grpc.testGatewayAccountID,
			TierName:  sdnmessage.ATierProfessional,
		},
		SecretHash: grpc.testGatewaySecretHash,
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "success with same accountID and secret hash",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
			},
			args: args{
				accountID:                    grpc.testGatewayAccountID,
				secretHash:                   grpc.testGatewaySecretHash,
				allowAccessToInternalGateway: false,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: nil,
		},
		{
			name: "invalid secret hash error",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
			},
			args: args{
				accountID:                    grpc.testGatewayAccountID,
				secretHash:                   grpc.testGatewaySecretHash + "1",
				allowAccessToInternalGateway: false,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: errInvalidHeader,
		},
		{
			name: "success with same accountID and empty secret hash",
			fields: fields{
				accountFetcher:  nil,
				sdnAccountModel: sdnAccountModelEnterprise,
			},
			args: args{
				accountID:                    grpc.testGatewayAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: false,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: nil,
		},
		{
			name: "deny access to internal gateway",
			fields: fields{
				accountFetcher:  nil,
				sdnAccountModel: sdnAccountModelEnterprise,
			},
			args: args{
				accountID:                    grpc.testDifferentAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: false,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: errMethodNotAllowed,
		},
		{
			name: "account manager not authorized error",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
				accountFetcher: func(accountID types.AccountID) (*accountResult, error) {
					return &accountResult{
						notAuthorizedErr: fmt.Errorf("not authorized"),
					}, nil
				},
			},
			args: args{
				accountID:                    grpc.testDifferentAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: true,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: fmt.Errorf("not authorized"),
		},
		{
			name: "success with enterprise account",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
				accountFetcher: func(accountID types.AccountID) (*accountResult, error) {
					return &accountResult{account: sdnAccountModelEnterprise}, nil
				},
			},
			args: args{
				accountID:                    grpc.testDifferentAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: true,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: nil,
		},
		{
			name: "tier to low error",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
				accountFetcher: func(accountID types.AccountID) (*accountResult, error) {
					return &accountResult{account: sdnAccountModelProfessional}, nil
				},
			},
			args: args{
				accountID:                    grpc.testDifferentAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: true,
				allowIntroductoryTierAccess:  false,
			},
			wantErr: errTierTooLow,
		},
		{
			name: "success with introductory tier access enabled",
			fields: fields{
				sdnAccountModel: sdnAccountModelEnterprise,
				accountFetcher: func(accountID types.AccountID) (*accountResult, error) {
					return &accountResult{account: sdnAccountModelProfessional}, nil
				},
			},
			args: args{
				accountID:                    grpc.testDifferentAccountID,
				secretHash:                   "",
				allowAccessToInternalGateway: true,
				allowIntroductoryTierAccess:  true,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl := gomock.NewController(t)
			mockedSdn := mock.NewMockSDNHTTP(ctl)

			g := &gateway{
				accountsCacheManager: utils.NewCache[types.AccountID, accountResult](syncmap.AccountIDHasher, tt.fields.accountFetcher, accountsCacheManagerExpDur, accountsCacheManagerCleanDur),
				log:                  log.Discard(),
				sdn:                  mockedSdn,
			}

			mockedSdn.EXPECT().AccountModel().Return(tt.fields.sdnAccountModel).AnyTimes()

			_, err := g.authorize(tt.args.accountID, tt.args.secretHash, tt.args.allowAccessToInternalGateway, tt.args.allowIntroductoryTierAccess, "")
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
*/
