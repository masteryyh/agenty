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

package actions

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

func listAllPages[T any](fetch func(page, pageSize int) (*pagination.PagedResponse[T], error)) ([]T, error) {
	page := 1
	items := make([]T, 0)

	for {
		result, err := fetch(page, 100)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return items, nil
		}

		items = append(items, result.Data...)
		if len(result.Data) == 0 || int64(len(items)) >= result.Total {
			return items, nil
		}
		page++
	}
}

func resolveByNameOrID[T any](nameOrID string, items []T, idOf func(T) uuid.UUID, nameOf func(T) string) (T, error) {
	var zero T

	value := strings.TrimSpace(nameOrID)
	if value == "" {
		return zero, fmt.Errorf("name or id is required")
	}

	if targetID, err := uuid.Parse(value); err == nil {
		for _, item := range items {
			if idOf(item) == targetID {
				return item, nil
			}
		}
		return zero, fmt.Errorf("resource not found: %s", value)
	}

	matches := make([]T, 0, 1)
	for _, item := range items {
		if nameOf(item) == value {
			matches = append(matches, item)
		}
	}

	switch len(matches) {
	case 0:
		return zero, fmt.Errorf("resource not found: %s", value)
	case 1:
		return matches[0], nil
	default:
		return zero, fmt.Errorf("resource name is ambiguous: %s; use id instead", value)
	}
}
