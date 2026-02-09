package casbin

import (
	"fmt"
	"os"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/casbin/casbin/v2/util"
)

type Config struct {
	ModelPath  string
	PolicyPath string
}

func LoadConfig() Config {
	modelPath := os.Getenv("CASBIN_MODEL_PATH")
	policyPath := os.Getenv("CASBIN_POLICY_PATH")

	if modelPath == "" {
		modelPath = "internal/auth/casbin/model.conf"
	}
	if policyPath == "" {
		policyPath = "internal/auth/casbin/policy.csv"
	}

	return Config{
		ModelPath:  modelPath,
		PolicyPath: policyPath,
	}
}

func NewEnforcer(cfg Config) (*casbin.Enforcer, error) {
	m, err := model.NewModelFromFile(cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("load casbin model (%s): %w", cfg.ModelPath, err)
	}

	a := fileadapter.NewAdapter(cfg.PolicyPath)

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}

	e.AddFunction("keyMatch2", func(args ...interface{}) (interface{}, error) {
		key1 := args[0].(string)
		key2 := args[1].(string)
		return util.KeyMatch2(key1, key2), nil
	})

	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("load casbin policy (%s): %w", cfg.PolicyPath, err)
	}

	return e, nil
}
