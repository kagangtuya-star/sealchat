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

type PresetSourceSpec struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	ProviderKind string `json:"providerKind"`
}

func PresetSourceCatalog() []PresetSourceSpec {
	return []PresetSourceSpec{
		{Key: "aliyun", Label: "阿里预设", ProviderKind: ProviderAliyun},
		{Key: "tencent", Label: "腾讯预设", ProviderKind: ProviderTencent},
		{Key: "tencent2", Label: "腾讯预设2", ProviderKind: ProviderTencent},
	}
}

func LookupProvider(kind string) (ProviderSpec, bool) {
	for _, spec := range ProviderCatalog() {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return ProviderSpec{}, false
}
