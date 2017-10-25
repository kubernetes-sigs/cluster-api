package terraformSDK

type Sdk struct {
}

func NewSdk() (*Sdk, error) {
	sdk := &Sdk{}
	return sdk, nil
}
