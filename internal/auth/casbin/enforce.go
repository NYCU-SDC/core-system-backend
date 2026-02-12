package casbin

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/casbin/casbin/v2/util"
)

type Config struct {
	ModelPath  string
	PolicyPath string
}

func NewEnforcer(cfg Config) (*casbin.Enforcer, error) {
	m, err := model.NewModelFromFile(cfg.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("load casbin model (%s): %w", cfg.ModelPath, err)
	}

	adapter := fileadapter.NewAdapter(cfg.PolicyPath)

	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}

	enforcer.AddFunction("keyMatch2", func(args ...interface{}) (interface{}, error) {
		key1 := args[0].(string)
		key2 := args[1].(string)
		return util.KeyMatch2(key1, key2), nil
	})

	err = enforcer.LoadPolicy()
	if err != nil {
		return nil, fmt.Errorf("load casbin policy (%s): %w", cfg.PolicyPath, err)
	}

	return enforcer, nil
}
