// Copyright 2021-present Thomas Meitz
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/rs/xid"
)

// XidMixin to be shared will all different schemas.
type XidMixin struct {
	mixin.Schema
}

// Fields of the User.
func (XidMixin) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			GoType(xid.ID{}).
			Comment("record identifier").
			DefaultFunc(xid.New),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Nillable().
			Comment("record creation date").
			Annotations(
			//entgql.OrderField("CREATED_AT"),
			).
			SchemaType(map[string]string{
				dialect.MySQL: "datetime(6)",
			}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Nillable().
			Comment("record update date").
			Annotations(
			// entgql.OrderField("UPDATED_AT"),
			).
			SchemaType(map[string]string{
				dialect.MySQL: "datetime(6)",
			}),
	}
}
