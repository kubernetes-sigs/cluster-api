package azuremaps

import (
	"fmt"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
)

func GetImageReferenceFromImage(image string) (*compute.ImageReference, error) {
	switch image {
	case "UbuntuServer 16.04.0-LTS":
		return &compute.ImageReference{
			Offer:     s("UbuntuServer"),
			Sku:       s("16.04-LTS"),
			Publisher: s("Canonical"),
			Version:   s("latest"),
		}, nil
	default:
		return nil, fmt.Errorf("Unable to find image reference for image [%s]", image)
	}
}

func s(st string) *string {
	return &st
}
