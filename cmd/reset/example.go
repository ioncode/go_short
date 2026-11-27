package main

// generate:reset
type ResetableStruct struct {
	i     int
	str   string
	strP  *string
	s     []int
	m     map[string]string
	child *ResetableStruct
}
