package casbin

import (
	"log"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/casbin/casbin/v2/util"
)

var Enforcer *casbin.Enforcer

func Init() {
	m, err := model.NewModelFromFile("internal/auth/casbin/model.conf")
	if err != nil {
		log.Fatal(err)
	}

	a := fileadapter.NewAdapter("internal/auth/casbin/policy.csv")

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		log.Fatal(err)
	}

	e.AddFunction("keyMatch2", func(args ...interface{}) (interface{}, error) {
		key1 := args[0].(string)
		key2 := args[1].(string)
		return util.KeyMatch2(key1, key2), nil
	})

	_ = e.LoadPolicy()
	Enforcer = e
}
