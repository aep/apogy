package server

import (
	"encoding/json"
	"fmt"
	openapi "github.com/aep/apogy/api/go"
	"strconv"
)

func Mutate(val map[string]interface{}, model *Model, mut *openapi.Mutations) (interface{}, error) {

	for k, mutationObj := range *mut {
		var op rune
		var newVal json.Number
		var opFound bool = false

		if mutationObj.Set != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = 's'
			val[k] = *mutationObj.Set
			continue
		}

		if mutationObj.Add != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = '+'
			if v, ok := processValue(*mutationObj.Add); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		// Handle Sub operation
		if mutationObj.Sub != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = '-'
			if v, ok := processValue(*mutationObj.Sub); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		// Handle Mul operation
		if mutationObj.Mul != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = '*'
			if v, ok := processValue(*mutationObj.Mul); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		// Handle Div operation
		if mutationObj.Div != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = '/'
			if v, ok := processValue(*mutationObj.Div); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		// Handle Min operation
		if mutationObj.Min != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = 'm'
			if v, ok := processValue(*mutationObj.Min); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		// Handle Max operation
		if mutationObj.Max != nil {
			if opFound {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}
			op = 'M'
			if v, ok := processValue(*mutationObj.Max); ok {
				newVal = v
				opFound = true
			} else {
				return nil, fmt.Errorf("invalid mutation value for '%s'", k)
			}
		}

		if !opFound {
			continue
		}

		// Get the current value to mutate
		oldVal, exists := val[k]
		if !exists {
			// If the field doesn't exist, we'll initialize it based on the operation
			oldVal = json.Number("0")
		}

		// Convert oldVal to json.Number if it's not already
		oldValNum, ok := oldVal.(json.Number)
		if !ok {
			return nil, fmt.Errorf("cannot perform mutation on non-numeric field '%s'", k)
		}

		if intVal, err := oldValNum.Int64(); err == nil {
			// Try to use integer math if the current value is an integer
			if mutIntVal, err := newVal.Int64(); err == nil {
				// Integer math
				var newIntVal int64
				switch op {
				case '+':
					newIntVal = intVal + mutIntVal
				case '-':
					newIntVal = intVal - mutIntVal
				case '*':
					newIntVal = intVal * mutIntVal
				case '/':
					if mutIntVal == 0 {
						return nil, fmt.Errorf("invalid mutation expression for '%s': division by zero", k)
					}
					newIntVal = intVal / mutIntVal
				case 'M':
					if mutIntVal > intVal {
						newIntVal = mutIntVal
					} else {
						newIntVal = intVal
					}
				case 'm':
					if mutIntVal < intVal {
						newIntVal = mutIntVal
					} else {
						newIntVal = intVal
					}
				}
				val[k] = json.Number(strconv.FormatInt(newIntVal, 10))
			} else {
				// If the mutation value is a float, we need to do float math
				floatVal := float64(intVal)
				mutFloatVal, err := newVal.Float64()
				if err != nil {
					return nil, fmt.Errorf("invalid mutation value for '%s': %v", k, err)
				}

				var newFloatVal float64
				switch op {
				case '+':
					newFloatVal = floatVal + mutFloatVal
				case '-':
					newFloatVal = floatVal - mutFloatVal
				case '*':
					newFloatVal = floatVal * mutFloatVal
				case '/':
					if mutFloatVal == 0 {
						return nil, fmt.Errorf("invalid mutation expression for '%s': division by zero", k)
					}
					newFloatVal = floatVal / mutFloatVal
				case 'M':
					if mutFloatVal > floatVal {
						newFloatVal = mutFloatVal
					} else {
						newFloatVal = floatVal
					}
				case 'm':
					if mutFloatVal < floatVal {
						newFloatVal = mutFloatVal
					} else {
						newFloatVal = floatVal
					}
				}
				val[k] = json.Number(strconv.FormatFloat(newFloatVal, 'f', -1, 64))
			}
		} else {
			// Float math
			floatVal, err := oldValNum.Float64()
			if err != nil {
				return nil, fmt.Errorf("invalid value for '%s': %v", k, err)
			}

			mutFloatVal, err := newVal.Float64()
			if err != nil {
				return nil, fmt.Errorf("invalid mutation value for '%s': %v", k, err)
			}

			var newFloatVal float64
			switch op {
			case '+':
				newFloatVal = floatVal + mutFloatVal
			case '-':
				newFloatVal = floatVal - mutFloatVal
			case '*':
				newFloatVal = floatVal * mutFloatVal
			case '/':
				if mutFloatVal == 0 {
					return nil, fmt.Errorf("invalid mutation expression for '%s': division by zero", k)
				}
				newFloatVal = floatVal / mutFloatVal
			case 'M':
				if mutFloatVal > floatVal {
					newFloatVal = mutFloatVal
				} else {
					newFloatVal = floatVal
				}
			case 'm':
				if mutFloatVal < floatVal {
					newFloatVal = mutFloatVal
				} else {
					newFloatVal = floatVal
				}
			}
			val[k] = json.Number(strconv.FormatFloat(newFloatVal, 'f', -1, 64))
		}
	}

	return val, nil
}

// processValue tries to convert various types to json.Number
func processValue(v interface{}) (json.Number, bool) {
	switch value := v.(type) {
	case json.Number:
		return value, true
	case string:
		return json.Number(value), true
	case float64:
		return json.Number(strconv.FormatFloat(value, 'f', -1, 64)), true
	case int64:
		return json.Number(strconv.FormatInt(value, 10)), true
	case int:
		return json.Number(strconv.Itoa(value)), true
	case float32:
		return json.Number(strconv.FormatFloat(float64(value), 'f', -1, 32)), true
	case int32:
		return json.Number(strconv.FormatInt(int64(value), 10)), true
	default:
		// Try to convert from map or struct to json.Number
		if numValue, err := json.Marshal(v); err == nil {
			var num json.Number
			if json.Unmarshal(numValue, &num) == nil {
				return num, true
			}
		}
		return json.Number(""), false
	}
}
