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

package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	vectorStoragePostgres = "postgres"
	vectorStorageSQLite   = "sqlite"
)

var vectorStorage = vectorStoragePostgres

type EmbeddingVector struct {
	vec []float32
}

func SetVectorStorage(storage string) {
	switch storage {
	case vectorStorageSQLite:
		vectorStorage = vectorStorageSQLite
	default:
		vectorStorage = vectorStoragePostgres
	}
}

func NewEmbeddingVector(vec []float32) EmbeddingVector {
	return EmbeddingVector{vec: vec}
}

func (v EmbeddingVector) Slice() []float32 {
	return v.vec
}

func (v EmbeddingVector) String() string {
	buf := make([]byte, 0, 2+16*len(v.vec))
	buf = append(buf, '[')
	for i, n := range v.vec {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendFloat(buf, float64(n), 'f', -1, 32)
	}
	buf = append(buf, ']')
	return string(buf)
}

func (v *EmbeddingVector) Scan(src any) error {
	switch src := src.(type) {
	case []byte:
		if len(src) > 0 && src[0] == '[' {
			return v.parseString(string(src))
		}
		return v.decodeFloat32Blob(src)
	case string:
		return v.parseString(src)
	default:
		return fmt.Errorf("unsupported embedding vector data type: %T", src)
	}
}

func (v EmbeddingVector) Value() (driver.Value, error) {
	if vectorStorage == vectorStorageSQLite {
		return v.encodeFloat32Blob(), nil
	}
	return v.String(), nil
}

func (v EmbeddingVector) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.vec)
}

func (v *EmbeddingVector) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &v.vec)
}

func (v *EmbeddingVector) parseString(s string) error {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return fmt.Errorf("invalid embedding vector")
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	if body == "" {
		v.vec = nil
		return nil
	}

	parts := strings.Split(body, ",")
	v.vec = make([]float32, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return err
		}
		v.vec = append(v.vec, float32(n))
	}
	return nil
}

func (v EmbeddingVector) encodeFloat32Blob() []byte {
	buf := make([]byte, len(v.vec)*4)
	for i, n := range v.vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(n))
	}
	return buf
}

func (v *EmbeddingVector) decodeFloat32Blob(buf []byte) error {
	if len(buf)%4 != 0 {
		return fmt.Errorf("invalid embedding vector blob length: %d", len(buf))
	}
	v.vec = make([]float32, len(buf)/4)
	for i := range v.vec {
		v.vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return nil
}

var _ sql.Scanner = (*EmbeddingVector)(nil)
var _ driver.Valuer = (*EmbeddingVector)(nil)
var _ json.Marshaler = (*EmbeddingVector)(nil)
var _ json.Unmarshaler = (*EmbeddingVector)(nil)
