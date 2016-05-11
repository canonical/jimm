// Copyright 2015 Canonical Ltd.

package jemcmd

type Patcher interface {
	PatchValue(dest, value interface{})
}

func PatchProviderDefaults(p Patcher, defaults map[string]map[string]func() (interface{}, error)) {
	defaults1 := make(map[string]map[string]func(schemaContext) (interface{}, error))
	for ptype, m := range defaults {
		m1 := make(map[string]func(schemaContext) (interface{}, error))
		defaults1[ptype] = m1
		for attr, f := range m {
			f := f
			m1[attr] = func(schemaContext) (interface{}, error) {
				return f()
			}
		}
	}
	p.PatchValue(&providerDefaults, defaults1)
}
