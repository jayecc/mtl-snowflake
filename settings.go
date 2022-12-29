package generator

import (
	"errors"
	"fmt"
	"time"
)

const (
	defaultTimeBit      uint64 = 41                                                             //时间位数(可使用64年)
	defaultMachineIDBit uint64 = 9                                                              //实例ID位数(512实例)
	defaultTimelineBit  uint64 = 1                                                              //时间线位数,处理时钟回退
	defaultSeqBit       uint64 = 63 - defaultTimeBit - defaultMachineIDBit - defaultTimelineBit //序号位数
	timeUnit            uint64 = 1e6                                                            //时间单位(1e6相当于ms)
	maxWaitTime         int64  = 1                                                              //当时间出现小幅回退时(这里设置为1时间单位)，等待时间递进到回退前时间再继续
)

var (
	DefaultEpoch int64 = time.Date(2020, 6, 10, 0, 0, 0, 0, time.UTC).UnixNano()
)

type Settings struct {
	TimeBit      uint64   //时间位长度
	MachineIDBit uint64   //实例ID位长度
	TimelineBit  uint64   //时间线位长度
	SeqBit       uint64   //序号位长度
	Epoch        int64    //时间位的基准时间(unix nano)
	presets      *presets //预先计算的参数
}

// presets 预先计算的参数
type presets struct {
	shiftTimeBit, shiftMachineIDBit, shiftTimelineBit, shiftSeq uint64
	maskTime, maskMachineID, maskTimeline, maskSeq              int64
	maxTime, maxMachineID, maxTimeline, maxSeq                  int64
}

var DefaultSettings = &Settings{
	TimeBit:      defaultTimeBit,
	MachineIDBit: defaultMachineIDBit,
	TimelineBit:  defaultTimelineBit,
	SeqBit:       defaultSeqBit,
	Epoch:        DefaultEpoch,
}

// calcPresets 计算预置参数
func calcPresets(settings *Settings) *presets {
	curPresets := new(presets)

	//移位位数
	curPresets.shiftSeq = 0
	curPresets.shiftTimelineBit = curPresets.shiftSeq + settings.SeqBit
	curPresets.shiftMachineIDBit = curPresets.shiftTimelineBit + settings.TimelineBit
	curPresets.shiftTimeBit = curPresets.shiftMachineIDBit + settings.MachineIDBit

	//最大值
	curPresets.maxSeq = (1 << settings.SeqBit) - 1
	curPresets.maxTimeline = (1 << settings.TimelineBit) - 1
	curPresets.maxMachineID = (1 << settings.MachineIDBit) - 1
	curPresets.maxTime = (1 << settings.TimeBit) - 1

	//掩码
	curPresets.maskSeq = ((1 << settings.SeqBit) - 1) << curPresets.shiftSeq
	curPresets.maskTimeline = ((1 << settings.TimelineBit) - 1) << curPresets.shiftTimelineBit
	curPresets.maskMachineID = ((1 << settings.MachineIDBit) - 1) << curPresets.shiftMachineIDBit
	curPresets.maskTime = ((1 << settings.TimeBit) - 1) << curPresets.shiftTimeBit
	return curPresets
}

// checkSettings 参数校验
func checkSettings(settings *Settings, machineID int64) error {
	if 63 != settings.TimeBit+settings.MachineIDBit+settings.TimelineBit+settings.SeqBit {
		return errors.New("TimeBit+MachineIDBit+TimelineBit+SeqBit !=63")
	}

	maxTime := int64((1 << settings.TimeBit) - 1)
	curTime := (time.Now().UnixNano() - settings.Epoch) / int64(timeUnit)

	if curTime < 0 {
		return errors.New("基准时间epoch须不晚于当前时间")
	}

	if curTime > maxTime {
		return errors.New("当前时间偏移量已超过最大限制，请设置更多的时间位数或设置一个更近的基准时间")
	}

	maxMachineID := (1 << settings.MachineIDBit) - 1
	if machineID < 0 || machineID > int64(maxMachineID) {
		return errors.New(fmt.Sprintf("machineID 必须介于0-%d(2^MachineIDBit-1)之间", maxMachineID))
	}
	return nil
}
