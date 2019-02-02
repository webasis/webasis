package webasis

import "strconv"

func Bool(v bool) string {
	if v {
		return "T"
	}
	return "F"
}

func Int(v int) string {
	return strconv.Itoa(v)
}

type Fields []string

func (vs Fields) Has(index int) bool {
	if index < len(vs) {
		return true
	}
	return false
}

func (vs Fields) Get(index int, defv string) string {
	if vs.Has(index) {
		return vs[index]
	}
	return defv
}

func (vs Fields) IsBool(index int) bool {
	v := vs.Get(index, "")
	return v == "T" || v == "F"
}

func (vs Fields) Bool(index int, defv bool) bool {
	if vs.IsBool(index) {
		return vs[index] == "T"
	}
	return defv
}

func (vs Fields) IsInt(index int) bool {
	v := vs.Get(index, "")
	_, err := strconv.Atoi(v)
	return err == nil
}

func (vs Fields) Int(index int, defv int) int {
	v := vs.Get(index, "")
	ret, err := strconv.Atoi(v)
	if err != nil {
		return defv
	}
	return ret
}
