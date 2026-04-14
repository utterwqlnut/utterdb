package server

import (
	"fmt"
	"strconv"
)

type Stringable interface {
	Stringify() string
}

type String struct {
	val string
}

type Int struct {
	val int
}

type Float struct {
	val float64
}

func (x String) Stringify() string {
	return x.val
}

func (x Int) Stringify() string {
	return strconv.Itoa(x.val)
}

func (x Float) Stringify() string {
	return strconv.FormatFloat(x.val, 'f', -1, 64)
}

func ParseToStringable(s string, sType string) (Stringable, error) {

	switch sType {

	case "int":
		val, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int: %w", err)
		}
		return &Int{val: val}, nil

	case "float":
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse float: %w", err)
		}
		return &Float{val: val}, nil

	case "string":
		return &String{val: s}, nil

	default:
		return nil, fmt.Errorf("invalid type: %s", sType)
	}
}
