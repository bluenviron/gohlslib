package primitives

// RemoveReturn removes trailing \n and \r
func RemoveReturn(s string) string {
	s = s[:len(s)-1]
	if len(s) != 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}
