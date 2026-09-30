package ttsprovider

const (
	ProviderAliyun  = "aliyun"
	ProviderTencent = "tencent"
)

// ProviderSpec describes a known brand, not implemented runtime capabilities.
type ProviderSpec struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func ProviderCatalog() []ProviderSpec {
	return []ProviderSpec{{Kind: ProviderAliyun, Name: "阿里"}, {Kind: ProviderTencent, Name: "腾讯"}}
}

func LookupProvider(kind string) (ProviderSpec, bool) {
	for _, spec := range ProviderCatalog() {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return ProviderSpec{}, false
}
