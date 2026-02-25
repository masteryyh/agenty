/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package routes

import (
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

type V1Routes struct {
	chatRoutes     *ChatRoutes
	providerRoutes *ProviderRoutes
	modelRoutes    *ModelRoutes
}

var (
	v1Routes *V1Routes
	v1Once   sync.Once
)

func GetV1Routes() *V1Routes {
	v1Once.Do(func() {
		v1Routes = &V1Routes{
			chatRoutes:     GetChatRoutes(),
			providerRoutes: GetProviderRoutes(),
			modelRoutes:    GetModelRoutes(),
		}
	})
	return v1Routes
}

func (r *V1Routes) RegisterRoutes(routerGroup *gin.RouterGroup) error {
	codeRegex := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		if err := v.RegisterValidation("code", func(fl validator.FieldLevel) bool {
			return codeRegex.MatchString(fl.Field().String())
		}); err != nil {
			return err
		}
	}

	r.chatRoutes.RegisterRoutes(routerGroup)
	r.providerRoutes.RegisterRoutes(routerGroup)
	r.modelRoutes.RegisterRoutes(routerGroup)
	return nil
}
