/*
Package volumetypes provides information and interaction with volume types in the
OpenStack Block Storage service. A volume type is a collection of specs used to
define the volume capabilities.

Example to list Volume Types

	allPages, err := volumetypes.List(client, volumetypes.ListOpts{}).AllPages()
	if err != nil{
		panic(err)
	}
	volumeTypes, err := volumetypes.ExtractVolumeTypes(allPages)
	if err != nil{
		panic(err)
	}
	for _,vt := range volumeTypes{
		fmt.Println(vt)
	}

Example to show a Volume Type

	typeID := "0fe36e73809d46aeae6705c39077b1b3"
	volumeType, err := volumetypes.Get(client, typeID).Extract()
	if err != nil{
		panic(err)
	}
	fmt.Println(volumeType)

Example to create a Volume Type

	volumeType, err := volumetypes.Create(client, volumetypes.CreateOpts{
		Name:"volume_type_001",
		IsPublic:true,
		Description:"description_001",
	}).Extract()
	if err != nil{
		panic(err)
	}
	fmt.Println(volumeType)

Example to delete a Volume TYpe

	typeID := "0fe36e73809d46aeae6705c39077b1b3"
	err := volumetypes.Delete(client, typeID).ExtractErr()
	if err != nil{
		panic(err)
	}
*/

package volumetypes
