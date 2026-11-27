package main

// generate:reset
type ResetableStruct struct {
	strP  *string
	m     map[string]string
	child *ResetableStruct
	str   string
	s     []int
	i     int
}
