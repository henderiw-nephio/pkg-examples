/*
Copyright 2023 Nephio.

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

package v1

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/nephio-project/nephio/krm-functions/lib/kubeobject"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// errors
	errKubeObjectNotInitialized = "KubeObject not initialized"
	CniVersion                  = "0.3.1"
	NadMode                     = "bridge"
	NadType                     = "static"
)

var (
	ConfigType = []string{"spec", "config"}
)

type NadConfig struct {
	CniVersion string          `json:"cniVersion"`
	Vlan       int             `json:"vlan"`
	Plugins    []PluginCniType `json:"plugins"`
}

type PluginCniType struct {
	Type         string       `json:"type"`
	Capabilities Capabilities `json:"capabilities"`
	Master       string       `json:"master"`
	Mode         string       `json:"mode"`
	Ipam         Ipam         `json:"ipam"`
}

type Capabilities struct {
	Ips bool `json:"ips"`
	Mac bool `json:"mac"`
}

type Ipam struct {
	Type      string      `json:"type"`
	Addresses []Addresses `json:"addresses"`
}

type Addresses struct {
	Address string `json:"address"`
	Gateway string `json:"gateway"`
}

// NewFromKubeObject creates a new parser interface
// It expects a *fn.KubeObject as input representing the serialized yaml file
func NewFromKubeObject(o *fn.KubeObject) (*Nad, error) {
	r, err := kubeobject.NewFromKubeObject[*nadv1.NetworkAttachmentDefinition](o)
	if err != nil {
		return nil, err
	}
	return &Nad{*r}, nil
}

// NewFromYAML creates a new parser interface
// It expects a raw byte slice as input representing the serialized yaml file
func NewFromYAML(b []byte) (*Nad, error) {
	r, err := kubeobject.NewFromYaml[*nadv1.NetworkAttachmentDefinition](b)
	if err != nil {
		return nil, err
	}
	return &Nad{*r}, nil
}

// NewFromGoStruct creates a new parser interface
// It expects a go struct representing the interface krm resource
func NewFromGoStruct(x *nadv1.NetworkAttachmentDefinition) (*Nad, error) {
	r, err := kubeobject.NewFromGoStruct[*nadv1.NetworkAttachmentDefinition](x)
	if err != nil {
		return nil, err
	}
	return &Nad{*r}, nil
}

type Nad struct {
	kubeobject.KubeObjectExt[*nadv1.NetworkAttachmentDefinition]
}

func (r *Nad) GetStringValue(fields ...string) string {
	if r == nil {
		return ""
	}
	s, ok, err := r.NestedString(fields...)
	if err != nil {
		return ""
	}
	if !ok {
		return ""
	}
	return s
}

func (r *Nad) GetBoolValue(fields ...string) bool {
	if r == nil {
		return false
	}
	b, ok, err := r.NestedBool(fields...)
	if err != nil {
		return false
	}
	if !ok {
		return false
	}
	return b
}

func (r *Nad) GetIntValue(fields ...string) int {
	if r == nil {
		return 0
	}
	i, ok, err := r.NestedInt(fields...)
	if err != nil {
		return 0
	}
	if !ok {
		return 0
	}
	return i
}

func (r *Nad) GetStringMap(fields ...string) map[string]string {
	if r == nil {
		return map[string]string{}
	}
	m, ok, err := r.NestedStringMap(fields...)
	if err != nil {
		return map[string]string{}
	}
	if !ok {
		return map[string]string{}
	}
	return m
}

// GetConfigSpec gets the spec attributes in the kubeobject according the go struct
func (r *Nad) GetConfigSpec() string {
	if r == nil {
		return ""
	}
	s, ok, err := r.NestedString(ConfigType...)
	if err != nil {
		return ""
	}
	if !ok {
		return ""
	}
	return s
}

func (r *Nad) GetCNIType() string {
	nadConfigStruct := NadConfig{}
	if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
		return ""
	}
	return nadConfigStruct.Plugins[0].Type
}

func (r *Nad) GetVlan() int {
	nadConfigStruct := NadConfig{}
	if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
		return 0
	}
	return nadConfigStruct.Vlan
}

func (r *Nad) GetNadMaster() string {
	nadConfigStruct := NadConfig{}
	if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
		return ""
	}
	return nadConfigStruct.Plugins[0].Master
}

func (r *Nad) GetIpamAddress() []Addresses {
	nadConfigStruct := NadConfig{}
	if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
		return []Addresses{}
	}
	return nadConfigStruct.Plugins[0].Ipam.Addresses
}

// SetConfigSpec sets the spec attributes in the kubeobject according the go struct
func (r *Nad) SetConfigSpec(spec *nadv1.NetworkAttachmentDefinitionSpec) error {
	b, err := json.Marshal(spec.Config)
	if err != nil {
		return err
	}
	return r.SetNestedString(string(b), ConfigType...)
}

func (r *Nad) SetCNIType(cniType string) error {
	if cniType != "" {
		nadConfigStruct := NadConfig{
			Plugins: []PluginCniType{{}},
		}
		/*
		r.NestedSubObject()
		if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
			panic(err)
		}
		*/
		nadConfigStruct.Plugins[0].Type = cniType
		b, err := json.Marshal(nadConfigStruct)
		if err != nil {
			return err
		}
		return r.SetNestedString(string(b), ConfigType...)
	} else {
		return fmt.Errorf("unknown cniType")
	}
}

func (r *Nad) SetVlan(vlanType int) error {
	if vlanType != 0 {
		nadConfigStruct := NadConfig{}
		if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
			panic(err)
		}
		nadConfigStruct.Vlan = vlanType
		b, err := json.Marshal(nadConfigStruct)
		if err != nil {
			return err
		}
		return r.SetNestedString(string(b), ConfigType...)
	} else {
		return fmt.Errorf("unknown vlanType")
	}
}

func (r *Nad) SetNadMaster(nadMaster string) error {
	if nadMaster != "" {
		nadConfigStruct := NadConfig{}
		if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
			panic(err)
		}
		nadConfigStruct.Plugins[0].Master = nadMaster
		b, err := json.Marshal(nadConfigStruct)
		if err != nil {
			return err
		}
		return r.SetNestedString(string(b), ConfigType...)
	} else {
		return fmt.Errorf("unknown cniType")
	}
}

func (r *Nad) SetIpamAddress(ipam []Addresses) error {
	if ipam != nil {
		nadConfigStruct := NadConfig{}
		if err := json.Unmarshal([]byte(r.GetStringValue(ConfigType...)), &nadConfigStruct); err != nil {
			panic(err)
		}
		nadConfigStruct.Plugins[0].Ipam.Addresses = ipam
		b, err := json.Marshal(nadConfigStruct)
		if err != nil {
			return err
		}
		return r.SetNestedString(string(b), ConfigType...)
	} else {
		return fmt.Errorf("unknown cniType")
	}
}

func BuildNetworkAttachementDefinition(meta metav1.ObjectMeta, spec nadv1.NetworkAttachmentDefinitionSpec) *nadv1.NetworkAttachmentDefinition {
	return &nadv1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nadv1.SchemeGroupVersion.Identifier(),
			Kind:       reflect.TypeOf(nadv1.NetworkAttachmentDefinition{}).Name(),
		},
		ObjectMeta: meta,
		Spec:       spec,
	}
}
