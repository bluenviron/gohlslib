package gohlslib

type clientTimeConvFMP4 struct {
	leadingTimeScale int64
	leadingBaseTime  int64
}

func (ts *clientTimeConvFMP4) initialize() {
}

func (ts *clientTimeConvFMP4) convert(v int64, clockRate int) int64 {
	return v - multiplyAndDivide(ts.leadingBaseTime, int64(clockRate), ts.leadingTimeScale)
}
