package internal

import (
	"github.com/go-playground/validator/v10"
	"regexp"
)

func NewValidator() *validator.Validate {
	v := validator.New()
	
	_ = v.RegisterValidation("username_rules", func(fl validator.FieldLevel) bool {
        re := regexp.MustCompile(`^\w+$`)
        return re.MatchString(fl.Field().String())
    })

	return v
}

func ValidateStruct(v *validator.Validate, s interface{}) error {
	err := v.Struct(s)
	if err != nil {
		return err
	}
	return nil
}