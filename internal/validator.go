package internal

import (
	"NYCU-SDC/core-system-backend/internal/form/font"
	"github.com/go-playground/validator/v10"
	"regexp"
)

func NewValidator() *validator.Validate {
	v := validator.New()
	
	_ = v.RegisterValidation("username_rules", func(fl validator.FieldLevel) bool {
        re := regexp.MustCompile(`^\w+$`)
        return re.MatchString(fl.Field().String())
    })

	_ = v.RegisterValidation("font", func(fl validator.FieldLevel) bool {
		id := fl.Field().String()
		if id == "" {
			return true
		}
		ok, err := font.IsValidID(id)
		return err == nil && ok
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