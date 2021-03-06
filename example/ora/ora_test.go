package example

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/dsc"
	"github.com/viant/dsunit"
	"github.com/viant/endly"
	"github.com/viant/endly/system/docker"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/url"
	_ "gopkg.in/rana/ora.v4"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

/*
Prerequisites:
1.docker service running
2. localhost credentials  to conneect to the localhost vi SSH
	or generate ~/.secret/localhost.json with  endly -c=localhost option

	export CGO_CFLAGS=-I/opt/oracle/instantclient_12_1/sdk/include/
	export CGO_LDFLAGS=/opt/oracle/instantclient_12_1/lib/


3. Oracle instance client SDK.(client and sdk)
	Instant Client Package - Basic: All files required to run OCI, OCCI, and JDBC-OCI applications
	Instant Client Package - SDK: Additional header files and an example makefile for developing Oracle applications with Instant Client
4.  go get gopkg.in/rana/ora.v4
*/

//Global variables for all test integrating with endly.
var endlyManager = endly.New()
var endlyContext = endlyManager.NewContext(toolbox.NewContext())
var localhostCredential = path.Join(os.Getenv("HOME"), ".secret/localhost.json")

func init() {
	os.Setenv("ORACLE_HOME", "/opt/oracle/instantclient_12_1")
	os.Setenv("TNS_ADMIN", "/opt/oracle/instantclient_12_1/admin")
}

func setup(t *testing.T) {

	err := startOracle()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
}

func tearDown(t *testing.T) {
	err := stopOracle()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
}

func TestDsunit_Oracle(t *testing.T) {
	setup(t)
	defer tearDown(t)
	if dsunit.InitFromURL(t, "config/init.json") {
		if !dsunit.PrepareFor(t, "mydb", "data", "use_case_1") {
			return
		}
		err := runSomeBusinessLogic()
		if !assert.Nil(t, err) {
			return
		}
		dsunit.ExpectFor(t, "mydb", dsunit.FullTableDatasetCheckPolicy, "data", "use_case_1")
	}
}

func runSomeBusinessLogic() error {
	config, err := dsc.NewConfigWithParameters("ora", "mydb/oracle@127.0.0.1:1521/xe", "", nil)
	if err != nil {
		return err
	}
	manager, err := dsc.NewManagerFactory().Create(config)
	if err != nil {
		return err
	}
	result, err := manager.Execute("UPDATE users SET comments = ? WHERE id = ?", "dsunit test", 3)
	if err != nil {
		return err
	}
	sqlResult, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if sqlResult != 1 {
		return fmt.Errorf("expected one row updated but had: %v", sqlResult)
	}
	return nil
}

func startOracle() error {

	_, err := endlyManager.Run(endlyContext, &docker.RunRequest{
		Target: url.NewResource("ssh://127.0.0.1", localhostCredential),
		Image:  "wnameless/oracle-xe-11g:latest",
		Env: map[string]string{
			"ORACLE_ALLOW_REMOTE": "true",
		},
		Name: "ora_dsunit",
		Ports: map[string]string{
			"1521": "1521",
		},
	})
	if err != nil {
		return err
	}

	//it takes some time to docker container to fully start

	//user/passw@host:port/sid
	config, err := dsc.NewConfigWithParameters("ora", "system/oracle@127.0.0.1:1521/xe", "", nil)
	if err != nil {
		return err
	}

	dscManager, err := dsc.NewManagerFactory().Create(config)
	if err != nil {
		return err
	}
	defer dscManager.ConnectionProvider().Close()
	for i := 0; i < 60; i++ {
		var record = make(map[string]interface{})
		_, err = dscManager.ReadSingle(&record, "SELECT 1 AS name FROM dual", nil, nil)
		if err == nil {
			time.Sleep(2 * time.Second)
			break
		}
		//wait for docker service to fully start
		if !(strings.Contains(err.Error(), "TNS:connection closed") ||
			strings.Contains(err.Error(), "ORA-12537") ||
			strings.Contains(err.Error(), "ORA-12514") ||
			strings.Contains(err.Error(), "ORA-01033") ||
			strings.Contains(err.Error(), "bad connection")) {
			return err
		}
		time.Sleep(4 * time.Second)
	}
	return err
}

func stopOracle() error {
	_, err := endlyManager.Run(endlyContext, &docker.StopRequest{
		&docker.BaseRequest{
			Target: url.NewResource("ssh://127.0.0.1", localhostCredential),
			Name:   "ora_dsunit",
		},
	})
	if err != nil {
		return err
	}
	return err

}
