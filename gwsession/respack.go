package gwsession

import "crypto/md5"

// NewResourcePack builds the pack a deployment configured. Its id is
// vanilla's: UUID.nameUUIDFromBytes of the URL (an MD5 name-based UUID).
func NewResourcePack(url, sha1 string, required bool, prompt string) *ResourcePack {
	if url == "" {
		return nil
	}
	id := md5.Sum([]byte(url))
	id[6] = id[6]&0x0f | 0x30
	id[8] = id[8]&0x3f | 0x80
	return &ResourcePack{ID: id, URL: url, SHA1: sha1, Required: required, Prompt: prompt}
}
