package server

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func Mutate(val map[string]interface{}, model *Model, mut map[string]interface{}) (interface{}, error) {

	for k, v := range mut {

		mutExprMap, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid mutation expression for '%s': %T", k, v)
		}

		var op rune
		var newVal json.Number

		for k2, v2 := range mutExprMap {

			if op != rune(0) {
				return nil, fmt.Errorf("invalid mutation expression for '%s': only one op allowed", k)
			}

			switch k2 {
			case "add", "+":
				op = '+'
			case "sub", "subtract", "-":
				op = '-'
			case "mul", "multiply", "*":
				op = '*'
			case "div", "divide", "/":
				op = '/'
			case "min", "minimum":
				op = 'm'
			case "max", "maximum":
				op = 'M'
			case "set":
				op = 's'
			default:
				return nil, fmt.Errorf("invalid mutation expression for '%s': %s", k, k2)
			}

			if op == 's' {
				val[k] = v2
				continue
			} else {
				vv, ok := v2.(json.Number)
				if !ok {
					return nil, fmt.Errorf("invalid mutation value for '%s': %T", k, v2)
				}
				newVal = vv
			}
		}

		if op == rune(0) || op == 's' {
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
