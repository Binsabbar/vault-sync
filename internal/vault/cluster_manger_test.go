package vault

import (
	"context"
	"testing"
	"vault-sync/internal/config"
	"vault-sync/testutil"

	"github.com/stretchr/testify/suite"
)

type ClusterManagerTestSuite struct {
	suite.Suite
	vaultHelper *testutil.VaultHelper
	cfg         *config.VaultConfig
	approleName string
	approleId   string
	ctx         context.Context
}

func TestClusterManagerTestSuite(t *testing.T) {
	suite.Run(t, new(ClusterManagerTestSuite))
}

func (suite *ClusterManagerTestSuite) SetupTest() {
	var err error

	suite.ctx = context.Background()
	suite.vaultHelper, err = testutil.NewVaultClusterContainer(suite.T(), suite.ctx, "test-cluster")
	suite.Require().NoError(err)

	suite.vaultHelper.EnableAppRoleAuth(suite.ctx)
	roleID, roleSecret, err := suite.vaultHelper.CreateApproleWithRWPermissions(suite.ctx, "my-role", "my-mount")
	suite.Require().NoError(err, "Failed to create cluster manager")

	suite.cfg = &config.VaultConfig{
		Address:       suite.vaultHelper.Config.Address,
		AppRoleID:     roleID,
		AppRoleSecret: roleSecret,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}

	suite.approleId = roleID
	suite.approleName = "my-role"
}

func (suite *ClusterManagerTestSuite) TearDownTest() {
	if suite.vaultHelper != nil {
		suite.vaultHelper.Terminate(suite.ctx)
		suite.vaultHelper = nil
	}
}

func (suite *ClusterManagerTestSuite) TearDownSuite() {
	if suite.vaultHelper != nil {
		suite.vaultHelper.Terminate(suite.ctx)
	}
}

func (suite *ClusterManagerTestSuite) TestEnsureValidToken() {

	suite.Run("valid token does not return error", func() {
		clusterManager, _ := newClusterManager(suite.cfg)
		err := clusterManager.ensureValidToken(suite.ctx)
		suite.NoError(err)
	})

	suite.Run("refresh the token if it expires within 5 minutes", func() {
		suite.vaultHelper.SetTokenTTL(suite.ctx, suite.approleName, "4m", "10m")
		clusterManager, _ := newClusterManager(suite.cfg)
		clusterManager.authenticate(suite.ctx)

		err := clusterManager.ensureValidToken(suite.ctx)

		suite.NoError(err)
	})
}
