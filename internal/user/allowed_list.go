package user

import (
	"strings"
)

var AllowedOnboardingList map[string]struct{}

func InitAllowedList(config string){
	AllowedOnboardingList = make(map[string]struct{})
	parts := strings.Split(config, ",")
	for _, part := range parts {
		key := strings.TrimSpace(strings.ToLower(part))
		if key != ""{
			AllowedOnboardingList[key] = struct{}{}
		}
	}
}

func IsAllowed (email string) bool {
	_, exist := AllowedOnboardingList[strings.ToLower(email)]
	return exist
}