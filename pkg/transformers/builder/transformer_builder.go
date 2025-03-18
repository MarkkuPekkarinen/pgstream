// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"github.com/xataio/pgstream/pkg/transformers"
	"github.com/xataio/pgstream/pkg/transformers/greenmask"
	"github.com/xataio/pgstream/pkg/transformers/neosync"
)

func New(cfg *transformers.Config) (transformers.Transformer, error) {
	switch cfg.Name {
	case transformers.GreenmaskString:
		return greenmask.NewStringTransformer(cfg.Parameters)
	case transformers.GreenmaskFirstName:
		return greenmask.NewFirstNameTransformer(cfg.Parameters)
	case transformers.GreenmaskInteger:
		return greenmask.NewIntegerTransformer(cfg.Parameters)
	case transformers.GreenmaskFloat:
		return greenmask.NewFloatTransformer(cfg.Parameters)
	case transformers.GreenmaskUUID:
		return greenmask.NewUUIDTransformer(cfg.Parameters)
	case transformers.GreenmaskBoolean:
		return greenmask.NewBooleanTransformer(cfg.Parameters)
	case transformers.GreenmaskChoice:
		return greenmask.NewChoiceTransformer(cfg.Parameters)
	case transformers.GreenmaskUnixTimestamp:
		return greenmask.NewUnixTimestampTransformer(cfg.Parameters)
	case transformers.GreenmaskDate:
		return greenmask.NewDateTransformer(cfg.Parameters)
	case transformers.GreenmaskUTCTimestamp:
		return greenmask.NewUTCTimestampTransformer(cfg.Parameters)
	case transformers.String:
		return transformers.NewStringTransformer(cfg.Parameters)
	case transformers.NeosyncString:
		return neosync.NewStringTransformer(cfg.Parameters)
	case transformers.NeosyncFirstName:
		return neosync.NewFirstNameTransformer(cfg.Parameters)
	case transformers.NeosyncEmail:
		return neosync.NewEmailTransformer(cfg.Parameters)
	default:
		return nil, transformers.ErrUnsupportedTransformer
	}
}
