package main

import _ "embed"

const omcResourcePath = "/v0/resource/plugins/" + pluginStoreID + "/omc.json"

//go:embed assets/omc.json
var omcDescriptorJSON string
