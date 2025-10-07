package vault

import (
	"context"
	"fmt"
	"testing"
	"vault-sync/internal/config"
	"vault-sync/testutil"

	"github.com/stretchr/testify/suite"
)

type ClusterManagerTestSuite struct {
	suite.Suite
	vaultHelper *testutil.VaultHelper
	cfg         *config.VaultClusterConfig
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
	suite.vaultHelper, err = testutil.NewVaultClusterContainer(suite.ctx, "test-cluster")
	suite.Require().NoError(err)

	suite.vaultHelper.EnableAppRoleAuth(suite.ctx)
	roleID, roleSecret, err := suite.vaultHelper.CreateApproleWithRWPermissions(suite.ctx, "my-role", "my-mount")
	suite.Require().NoError(err, "Failed to create cluster manager")

	suite.cfg = &config.VaultClusterConfig{
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

	suite.Run("all methods check token before use", func() {
		type MethodsTestData struct {
			name         string
			invokeMethod func(cm *clusterManager) error
		}
		methodsToCheck := []MethodsTestData{
			{
				name: "checkMounts",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.checkMounts(suite.ctx, "main", []string{"my-mount"})
					return err
				},
			},
			{
				name: "fetchKeysUnderMount",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.fetchKeysUnderMount(suite.ctx, "my-mount", func(path string, isFinalPath bool) bool { return true })
					return err
				},
			},
			{
				name: "fetchSecretMetadata",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.fetchSecretMetadata(suite.ctx, "my-mount", "my-secret")
					return err
				},
			},
			{
				name: "secretExists",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.secretExists(suite.ctx, "my-mount", "my-secret")
					return err
				},
			},
			{
				name: "readSecret",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.readSecret(suite.ctx, "my-mount", "my-secret")
					return err
				},
			},
			{
				name: "writeSecret",
				invokeMethod: func(cm *clusterManager) error {
					_, err := cm.writeSecret(suite.ctx, "my-mount", "my-secret", map[string]interface{}{"key": "value"})
					return err
				},
			},
			{
				name: "deleteSecret",
				invokeMethod: func(cm *clusterManager) error {
					return cm.deleteSecret(suite.ctx, "my-mount", "my-secret")
				},
			},
		}
		for _, method := range methodsToCheck {
			suite.Run(fmt.Sprintf("returns error for method %s if token not valid", method.name), func() {
				clusterManager, _ := newClusterManager(suite.cfg)
				clusterManager.config.AppRoleID = "invalid-role"
				clusterManager.client.SetToken("invalid-token")

				err := method.invokeMethod(clusterManager)

				suite.Error(err, "Expected error when invoking method with invalid token")
				suite.Contains(err.Error(), "failed to authenticate", "Expected token invalid error")
			})
		}
	})
}
