package azurecommon

// TagsToMap flattens the Azure Resource Manager tag map, whose values are
// pointers, into the plain map the Docker API and sockerless's label
// reconstruction use. Nil values are dropped.
func TagsToMap(tags map[string]*string) map[string]string {
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}
