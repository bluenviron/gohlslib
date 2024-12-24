package primitives

import (
	"fmt"
	"strings"
)

// Attributes are playlist attributes.
type Attributes map[string]string

// Unmarshal decodes attributes.
func (a *Attributes) Unmarshal(v string) error {
	*a = make(Attributes)

	for {
		if len(v) == 0 {
			break
		}

		// read key
		i := strings.IndexByte(v, '=')
		if i < 0 {
			return fmt.Errorf("key not found")
		}
		var key string
		key, v = v[:i], v[i+1:]
		key = strings.TrimLeft(key, " ")

		// read value
		var val string
		if len(v) != 0 && v[0] == '"' {
			v = v[1:]
			i = strings.IndexByte(v, '"')
			if i < 0 {
				return fmt.Errorf("value end delimiter not found")
			}
			val, v = v[:i], v[i+1:]
			(*a)[key] = val

			if len(v) != 0 {
				if v[0] != ',' {
					return fmt.Errorf("delimiter not found")
				}
				v = v[1:]
			}
		} else {
			i = strings.IndexByte(v, ',')
			if i >= 0 {
				val, v = v[:i], v[i+1:]
				(*a)[key] = val
			} else {
				val = v
				(*a)[key] = val
				break
			}
		}
	}

	return nil
}
