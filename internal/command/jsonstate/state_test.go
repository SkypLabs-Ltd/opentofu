// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonstate

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestMarshalOutputs(t *testing.T) {
	tests := []struct {
		Outputs map[string]*states.OutputValue
		Want    map[string]Output
		Err     bool
	}{
		{
			nil,
			nil,
			false,
		},
		{
			map[string]*states.OutputValue{
				"test": {
					Sensitive: true,
					Value:     cty.StringVal("sekret"),
				},
			},
			map[string]Output{
				"test": {
					Sensitive: true,
					Value:     json.RawMessage(`"sekret"`),
					Type:      json.RawMessage(`"string"`),
				},
			},
			false,
		},
		{
			map[string]*states.OutputValue{
				"test": {
					Sensitive: false,
					Value:     cty.StringVal("not_so_sekret"),
				},
			},
			map[string]Output{
				"test": {
					Sensitive: false,
					Value:     json.RawMessage(`"not_so_sekret"`),
					Type:      json.RawMessage(`"string"`),
				},
			},
			false,
		},
		{
			map[string]*states.OutputValue{
				"mapstring": {
					Sensitive: false,
					Value: cty.MapVal(map[string]cty.Value{
						"beep": cty.StringVal("boop"),
					}),
				},
				"setnumber": {
					Sensitive: false,
					Value: cty.SetVal([]cty.Value{
						cty.NumberIntVal(3),
						cty.NumberIntVal(5),
						cty.NumberIntVal(7),
						cty.NumberIntVal(11),
					}),
				},
			},
			map[string]Output{
				"mapstring": {
					Sensitive: false,
					Value:     json.RawMessage(`{"beep":"boop"}`),
					Type:      json.RawMessage(`["map","string"]`),
				},
				"setnumber": {
					Sensitive: false,
					Value:     json.RawMessage(`[3,5,7,11]`),
					Type:      json.RawMessage(`["set","number"]`),
				},
			},
			false,
		},
		{
			map[string]*states.OutputValue{
				"foo": {
					Value:      cty.StringVal("bar"),
					Deprecated: "I am deprecated",
				},
			},
			map[string]Output{
				"foo": {
					Value:      json.RawMessage(`"bar"`),
					Type:       json.RawMessage(`"string"`),
					Deprecated: "I am deprecated",
				},
			},
			false,
		},
	}

	for _, test := range tests {
		got, err := MarshalOutputs(test.Outputs)
		if test.Err {
			if err == nil {
				t.Fatal("succeeded; want error")
			}
			return
		} else if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if !cmp.Equal(test.Want, got) {
			t.Fatalf("wrong result:\n%s", cmp.Diff(test.Want, got))
		}
	}
}

func TestMarshalAttributeValues(t *testing.T) {
	tests := []struct {
		Attr cty.Value
		Want AttributeValues
	}{
		{
			cty.NilVal,
			nil,
		},
		{
			cty.NullVal(cty.String),
			nil,
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			AttributeValues{"foo": json.RawMessage(`"bar"`)},
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.String),
			}),
			AttributeValues{"foo": json.RawMessage(`null`)},
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"bar": cty.MapVal(map[string]cty.Value{
					"hello": cty.StringVal("world"),
				}),
				"baz": cty.ListVal([]cty.Value{
					cty.StringVal("goodnight"),
					cty.StringVal("moon"),
				}),
			}),
			AttributeValues{
				"bar": json.RawMessage(`{"hello":"world"}`),
				"baz": json.RawMessage(`["goodnight","moon"]`),
			},
		},
		// Marked values
		{
			cty.ObjectVal(map[string]cty.Value{
				"bar": cty.MapVal(map[string]cty.Value{
					"hello": cty.StringVal("world"),
				}),
				"baz": cty.ListVal([]cty.Value{
					cty.StringVal("goodnight"),
					cty.StringVal("moon").Mark(marks.Sensitive),
				}),
			}),
			AttributeValues{
				"bar": json.RawMessage(`{"hello":"world"}`),
				"baz": json.RawMessage(`["goodnight","moon"]`),
			},
		},
	}

	for _, test := range tests {
		got := marshalAttributeValues(test.Attr)
		eq := reflect.DeepEqual(got, test.Want)
		if !eq {
			t.Fatalf("wrong result:\nGot: %#v\nWant: %#v\n", got, test.Want)
		}
	}
}

func TestMarshalResources(t *testing.T) {
	deposedKey := states.NewDeposedKey()
	tests := map[string]struct {
		Resources map[string]*states.Resource
		Schemas   *tofu.Schemas
		Want      []Resource
		Err       bool
	}{
		"nil": {
			nil,
			nil,
			nil,
			false,
		},
		"single resource": {
			map[string]*states.Resource{
				"test_thing.baz": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"woozles":"confuzles"}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"single resource_with_sensitive": {
			map[string]*states.Resource{
				"test_thing.baz": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"woozles":"confuzles","foozles":"sensuzles"}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`"sensuzles"`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"resource with marks": {
			map[string]*states.Resource{
				"test_thing.bar": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"foozles":"confuzles"}`),
								AttrSensitivePaths: []cty.PathValueMarks{{
									Path:  cty.Path{cty.GetAttrStep{Name: "foozles"}},
									Marks: cty.NewValueMarks(marks.Sensitive)},
								},
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`"confuzles"`),
						"woozles": json.RawMessage(`null`),
					},
					SensitiveValues: json.RawMessage(`{"foozles":true}`),
				},
			},
			false,
		},
		"single resource wrong schema": {
			map[string]*states.Resource{
				"test_thing.baz": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: &states.ResourceInstanceObjectSrc{
								SchemaVersion: 1,
								Status:        states.ObjectReady,
								AttrsJSON:     []byte(`{"woozles":["confuzles"]}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			nil,
			true,
		},
		"resource with count": {
			map[string]*states.Resource{
				"test_thing.bar": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.IntKey(0): {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"woozles":"confuzles"}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar[0]",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        json.RawMessage(`0`),
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"resource with for_each": {
			map[string]*states.Resource{
				"test_thing.bar": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.StringKey("rockhopper"): {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"woozles":"confuzles"}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar[\"rockhopper\"]",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        json.RawMessage(`"rockhopper"`),
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"deposed resource": {
			map[string]*states.Resource{
				"test_thing.baz": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Deposed: map[states.DeposedKey]*states.ResourceInstanceObjectSrc{
								states.DeposedKey(deposedKey): {
									Status:    states.ObjectReady,
									AttrsJSON: []byte(`{"woozles":"confuzles"}`),
								},
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					DeposedKey:   deposedKey.String(),
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"deposed and current resource": {
			map[string]*states.Resource{
				"test_thing.baz": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Deposed: map[states.DeposedKey]*states.ResourceInstanceObjectSrc{
								states.DeposedKey(deposedKey): {
									Status:    states.ObjectReady,
									AttrsJSON: []byte(`{"woozles":"confuzles"}`),
								},
							},
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"woozles":"confuzles"}`),
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
				{
					Address:      "test_thing.bar",
					Mode:         "managed",
					Type:         "test_thing",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					DeposedKey:   deposedKey.String(),
					AttributeValues: AttributeValues{
						"foozles": json.RawMessage(`null`),
						"woozles": json.RawMessage(`"confuzles"`),
					},
					SensitiveValues: json.RawMessage("{\"foozles\":true}"),
				},
			},
			false,
		},
		"resource with marked map attr": {
			map[string]*states.Resource{
				"test_map_attr.bar": {
					Addr: addrs.AbsResource{
						Resource: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_map_attr",
							Name: "bar",
						},
					},
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: &states.ResourceInstanceObjectSrc{
								Status:    states.ObjectReady,
								AttrsJSON: []byte(`{"data":{"woozles":"confuzles"}}`),
								AttrSensitivePaths: []cty.PathValueMarks{{
									Path:  cty.Path{cty.GetAttrStep{Name: "data"}},
									Marks: cty.NewValueMarks(marks.Sensitive)},
								},
							},
						},
					},
					ProviderConfig: addrs.AbsProviderConfig{
						Provider: addrs.NewDefaultProvider("test"),
						Module:   addrs.RootModule,
					},
				},
			},
			testSchemas(),
			[]Resource{
				{
					Address:      "test_map_attr.bar",
					Mode:         "managed",
					Type:         "test_map_attr",
					Name:         "bar",
					Index:        nil,
					ProviderName: "registry.opentofu.org/hashicorp/test",
					AttributeValues: AttributeValues{
						"data": json.RawMessage(`{"woozles":"confuzles"}`),
					},
					SensitiveValues: json.RawMessage(`{"data":true}`),
				},
			},
			false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := marshalResources(test.Resources, addrs.RootModuleInstance, test.Schemas)
			if test.Err {
				if err == nil {
					t.Fatal("succeeded; want error")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			diff := cmp.Diff(got, test.Want)
			if diff != "" {
				t.Fatalf("wrong result: %s\n", diff)
			}

		})
	}
}

func TestMarshalModules_basic(t *testing.T) {
	childModule, _ := addrs.ParseModuleInstanceStr("module.child")
	subModule, _ := addrs.ParseModuleInstanceStr("module.submodule")
	testState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(childModule),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   childModule.Module(),
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(subModule),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   subModule.Module(),
			},
			addrs.NoKey,
		)
	})
	moduleMap := make(map[string][]addrs.ModuleInstance)
	moduleMap[""] = []addrs.ModuleInstance{childModule, subModule}

	got, err := marshalModules(testState, testSchemas(), moduleMap[""], moduleMap)

	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(got) != 2 {
		t.Fatalf("wrong result! got %d modules, expected 2", len(got))
	}

	if got[0].Address != "module.child" || got[1].Address != "module.submodule" {
		t.Fatalf("wrong result! got %#v\n", got)
	}

}

func TestMarshalModules_nested(t *testing.T) {
	childModule, _ := addrs.ParseModuleInstanceStr("module.child")
	subModule, _ := addrs.ParseModuleInstanceStr("module.child.module.submodule")
	testState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(childModule),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   childModule.Module(),
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(subModule),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   subModule.Module(),
			},
			addrs.NoKey,
		)
	})
	moduleMap := make(map[string][]addrs.ModuleInstance)
	moduleMap[""] = []addrs.ModuleInstance{childModule}
	moduleMap[childModule.String()] = []addrs.ModuleInstance{subModule}

	got, err := marshalModules(testState, testSchemas(), moduleMap[""], moduleMap)

	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(got) != 1 {
		t.Fatalf("wrong result! got %d modules, expected 1", len(got))
	}

	if got[0].Address != "module.child" {
		t.Fatalf("wrong result! got %#v\n", got)
	}

	if got[0].ChildModules[0].Address != "module.child.module.submodule" {
		t.Fatalf("wrong result! got %#v\n", got)
	}
}

func TestMarshalModules_parent_no_resources(t *testing.T) {
	subModule, _ := addrs.ParseModuleInstanceStr("module.child.module.submodule")
	testState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(subModule),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"foo","foo":"value","bar":"value"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   subModule.Module(),
			},
			addrs.NoKey,
		)
	})
	got, err := marshalRootModule(testState, testSchemas())

	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(got.ChildModules) != 1 {
		t.Fatalf("wrong result! got %d modules, expected 1", len(got.ChildModules))
	}

	if got.ChildModules[0].Address != "module.child" {
		t.Fatalf("wrong result! got %#v\n", got)
	}

	if got.ChildModules[0].ChildModules[0].Address != "module.child.module.submodule" {
		t.Fatalf("wrong result! got %#v\n", got)
	}
}

func testSchemas() *tofu.Schemas {
	return &tofu.Schemas{
		Providers: map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): {
				ResourceTypes: map[string]providers.Schema{
					"test_thing": {
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"woozles": {Type: cty.String, Optional: true, Computed: true},
								"foozles": {Type: cty.String, Optional: true, Sensitive: true},
							},
						},
					},
					"test_instance": {
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"id":  {Type: cty.String, Optional: true, Computed: true},
								"foo": {Type: cty.String, Optional: true},
								"bar": {Type: cty.String, Optional: true},
							},
						},
					},
					"test_map_attr": {
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"data": {Type: cty.Map(cty.String), Optional: true, Computed: true, Sensitive: true},
							},
						},
					},
				},
			},
		},
	}
}

func TestSensitiveAsBool(t *testing.T) {
	tests := []struct {
		Input cty.Value
		Want  cty.Value
	}{
		{
			cty.StringVal("hello"),
			cty.False,
		},
		{
			cty.NullVal(cty.String),
			cty.False,
		},
		{
			cty.StringVal("hello").Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.NullVal(cty.String).Mark(marks.Sensitive),
			cty.True,
		},

		{
			cty.NullVal(cty.DynamicPseudoType).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.NullVal(cty.Object(map[string]cty.Type{"test": cty.String})),
			cty.False,
		},
		{
			cty.NullVal(cty.Object(map[string]cty.Type{"test": cty.String})).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.DynamicVal,
			cty.False,
		},
		{
			cty.DynamicVal.Mark(marks.Sensitive),
			cty.True,
		},

		{
			cty.ListValEmpty(cty.String),
			cty.EmptyTupleVal,
		},
		{
			cty.ListValEmpty(cty.String).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
				cty.StringVal("friend").Mark(marks.Sensitive),
			}),
			cty.TupleVal([]cty.Value{
				cty.False,
				cty.True,
			}),
		},
		{
			cty.SetValEmpty(cty.String),
			cty.EmptyTupleVal,
		},
		{
			cty.SetValEmpty(cty.String).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.SetVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.SetVal([]cty.Value{cty.StringVal("hello").Mark(marks.Sensitive)}),
			cty.True,
		},
		{
			cty.EmptyTupleVal.Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("hello"),
				cty.StringVal("friend").Mark(marks.Sensitive),
			}),
			cty.TupleVal([]cty.Value{
				cty.False,
				cty.True,
			}),
		},
		{
			cty.MapValEmpty(cty.String),
			cty.EmptyObjectVal,
		},
		{
			cty.MapValEmpty(cty.String).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.MapVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse"),
			}),
			cty.EmptyObjectVal,
		},
		{
			cty.MapVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse").Mark(marks.Sensitive),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"animal": cty.True,
			}),
		},
		{
			cty.MapVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse").Mark(marks.Sensitive),
			}).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.EmptyObjectVal,
			cty.EmptyObjectVal,
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse"),
			}),
			cty.EmptyObjectVal,
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse").Mark(marks.Sensitive),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"animal": cty.True,
			}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse").Mark(marks.Sensitive),
			}).Mark(marks.Sensitive),
			cty.True,
		},
		{
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known").Mark(marks.Sensitive),
				}),
			}),
			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
			}),
		},
		{
			cty.ListVal([]cty.Value{
				cty.MapValEmpty(cty.String),
				cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("known").Mark(marks.Sensitive),
				}),
				cty.MapVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
			}),
			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
				cty.EmptyObjectVal,
			}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"list":   cty.UnknownVal(cty.List(cty.String)),
				"set":    cty.UnknownVal(cty.Set(cty.Bool)),
				"tuple":  cty.UnknownVal(cty.Tuple([]cty.Type{cty.String, cty.Number})),
				"map":    cty.UnknownVal(cty.Map(cty.String)),
				"object": cty.UnknownVal(cty.Object(map[string]cty.Type{"a": cty.String})),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"list":   cty.EmptyTupleVal,
				"set":    cty.EmptyTupleVal,
				"tuple":  cty.EmptyTupleVal,
				"map":    cty.EmptyObjectVal,
				"object": cty.EmptyObjectVal,
			}),
		},
	}

	for _, test := range tests {
		got := SensitiveAsBool(test.Input)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf(
				"wrong result\ninput: %#v\ngot:   %#v\nwant:  %#v",
				test.Input, got, test.Want,
			)
		}
	}
}

func TestSensitiveAsBoolWithPathValueMarks(t *testing.T) {
	tests := []struct {
		Input cty.Value
		Pvms  []cty.PathValueMarks
		Want  cty.Value
	}{
		{
			cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
				cty.StringVal("friend"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.False,
				cty.True,
			}),
		},
		{
			cty.ListVal([]cty.Value{
				cty.StringVal("hello").Mark(marks.Sensitive),
				cty.StringVal("friend"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.True,
				cty.True,
			}),
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("hello"),
				cty.StringVal("friend"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.False,
				cty.True,
			}),
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("hello").Mark(marks.Sensitive),
				cty.StringVal("friend"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.True,
				cty.True,
			}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.StringVal("hello"),
				cty.StringVal("friend"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.StringVal("hello")}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.False,
				cty.True,
			}),
		},
		{
			cty.MapVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.StringVal("animal")}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.ObjectVal(map[string]cty.Value{
				"animal": cty.True,
			}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"greeting": cty.StringVal("hello"),
				"animal":   cty.StringVal("horse"),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.GetAttrStep{Name: "animal"}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.ObjectVal(map[string]cty.Value{
				"animal": cty.True,
			}),
		},
		{
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}, cty.GetAttrStep{Name: "a"}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},

			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
			}),
		},
		{
			cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}, cty.GetAttrStep{Name: "a"}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},

			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
			}),
		},
		{
			cty.ListVal([]cty.Value{
				cty.MapValEmpty(cty.String),
				cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
				cty.MapVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
			}),
			[]cty.PathValueMarks{{
				Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(1)}, cty.IndexStep{Key: cty.StringVal("a")}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
				cty.EmptyObjectVal,
			}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
					"animal":   cty.StringVal("cat"),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
					"animal":   cty.StringVal("horse"),
				}),
			}),
			[]cty.PathValueMarks{{
				Path: cty.Path{
					cty.IndexStep{Key: cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("hello"),
						"animal":   cty.StringVal("cat"),
					})},
					cty.GetAttrStep{Name: "animal"}},
				Marks: cty.NewValueMarks(marks.Sensitive)},
			},

			cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"animal": cty.True,
				}),
				cty.EmptyObjectVal,
			}),
		},
	}

	for _, test := range tests {
		got := SensitiveAsBoolWithPathValueMarks(test.Input, test.Pvms)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf(
				"wrong result\ninput: %#v\ngot:   %#v\nwant:  %#v",
				test.Input, got, test.Want,
			)
		}
	}
}
