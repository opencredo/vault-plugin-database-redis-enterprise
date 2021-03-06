package plugin

import (
	"context"

	"github.com/RedisLabs/vault-plugin-database-redisenterprise/internal/sdk"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"testing"
	"time"

	"github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	dbtesting "github.com/hashicorp/vault/sdk/database/dbplugin/v5/testing"
)

func TestRedisEnterpriseDB_UpdateUser_With_New_Password(t *testing.T) {
	db := setupRedisEnterpriseDB(t, "", false)

	createReq := dbplugin.NewUserRequest{
		UsernameConfig: dbplugin.UsernameMetadata{
			DisplayName: "tester_update",
			RoleName:    "test",
		},
		Statements: dbplugin.Statements{
			Commands: []string{`{"role":"DB Member"}`},
		},
		Password:   "testing",
		Expiration: time.Now().Add(time.Minute),
	}

	userResponse := dbtesting.AssertNewUser(t, db, createReq)

	client := sdk.NewClient(hclog.Default())
	client.Initialise(url, username, password)

	beforeUpdate, err := client.FindUserByName(context.TODO(), userResponse.Username)
	require.NoError(t, err)

	// Wait a bit so the password updated date will be different
	time.Sleep(2 * time.Second)

	updateReq := dbplugin.UpdateUserRequest{
		Username: userResponse.Username,
		Password: &dbplugin.ChangePassword{
			NewPassword: "xyzzyxyzzy",
		},
	}

	dbtesting.AssertUpdateUser(t, db, updateReq)

	afterUpdate, err := client.FindUserByName(context.TODO(), userResponse.Username)
	require.NoError(t, err)

	assert.NotEqual(t, beforeUpdate.PasswordIssueDate, afterUpdate.PasswordIssueDate)

	teardownUserFromDatabase(t, db, userResponse.Username)
}
