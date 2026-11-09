package main

import _ "embed"

const descriptorResourcePath = "/v0/resource/plugins/" + pluginStoreID + "/doujie.json"

//go:embed assets/doujie.json
var descriptorJSON string
