package casbin

import (
	"log"
	"os"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/casbin/casbin/v2/util"
)

var Enforcer *casbin.Enforcer

func Init() {
	modelPath := os.Getenv("CASBIN_MODEL_PATH")
	policyPath := os.Getenv("CASBIN_POLICY_PATH")

	if modelPath == "" {
		modelPath = "internal/auth/casbin/model.conf"
	}
	if policyPath == "" {
		policyPath = "internal/auth/casbin/policy.csv"
	}

	m, err := model.NewModelFromFile(modelPath)
	if err != nil {
		log.Fatalf("failed to load casbin model (%s): %v", modelPath, err)
	}

	a := fileadapter.NewAdapter(policyPath)

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		log.Fatalf("failed to create casbin enforcer: %v", err)
	}

	e.AddFunction("keyMatch2", func(args ...interface{}) (interface{}, error) {
		key1 := args[0].(string)
		key2 := args[1].(string)
		return util.KeyMatch2(key1, key2), nil
	})

	_ = e.LoadPolicy()
	Enforcer = e
}
