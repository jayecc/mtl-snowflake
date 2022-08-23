// mtl-snowflake(multi-timeline-snowflake) 基于多时间线改进的snowflake分布式id生成器
//
// snowflake(雪花)算法是twitter开源的分布式id生成算法，主要优点有：
//   - 极少依赖(仅依赖系统时钟)
//   - 分布式全局唯一
//   - 满足趋势递增
//   - 理论上限(推荐设置)：单机每秒可生成409.6万ID(集群42亿/s)
//
//                           snowflake 推荐id结构
//
//   /-- 符号位 --\ /-------时间戳--------\ /--机器id- -\ /---序列号----\
//  |      0       |------   41bit   ------|--  10bit  --|---  12bit  ---|
//
//                            mtl-snowflake 推荐id结构
//
//   /-- 符号位 --\ /-------时间戳--------\ /--机器id- -\ /---时间线--\ /---序列号----\
//  |      0       |------   41bit   ------|--   9bit  --|---  1bit  --|---  12bit  ---|
//
//  mtl-snowflake在snowflake id结构中增加时间线部分(推荐1位)，通过设置多条时间线，来解决当发生时钟回退时的id重复问题。
//    - step1 初始时，算法随机选定一条时间线作为当前时间线，生成id的同时推进当前时间线进度。
//    - step2 当发生时钟回退，算法暂停当前时间线进度，选择一条合适的时间线(进度<当前时间)并切换到该时间线，这样算法可以继续生成不重复的ID
//
//  mtl-snowflake：
//    - 时钟回退情况下仍能保证id全局唯一
//    - 理论上限(推荐设置)：单机每秒可生成409.6万ID(集群21亿/s)
//

package generator

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type IDGenerator struct {
	mutex            *sync.Mutex //互斥锁，保证线程安全
	settings         *Settings   //生成器参数
	timelineProgress []int64     //各时间线进度
	curTimeline      int64       //当前时间线
	seq              int64       //当前序号
	machineID        int64       //节点编号
}

// ID结构
type IDCompose struct {
	Time      int64 //时间单位ms
	MachineID int64 //机器ID
	TimeLine  int64 //时间线
	Seq       int64 //序号
}

// GetMachineID 节点编号
func (idGen *IDGenerator) GetMachineID() int64 {
	return idGen.machineID
}

// GetSettings 初始化配置
func (idGen *IDGenerator) GetSettings() Settings {
	return *idGen.settings
}

// NewGenerator 创建一个id生成器
//   - TimeBit=41 可使用64年
//   - MachineIDBit=9 最多512个节点
//   - TimelineBit=1  两条时间线，能解决常见的时间回退问题
//   - SeqBit=12 1毫秒内最多生成4096个序号
//   - Epoch=1433865600000000000(2015.6.10 00:00:00) 基准时间(unix nano)
func NewGenerator(machineID int64) (*IDGenerator, error) {
	return NewGeneratorWithSettings(machineID, *DefaultSettings)
}

// NewGeneratorWithSettings 创建一个id生成器
func NewGeneratorWithSettings(machineID int64, settings Settings) (*IDGenerator, error) {
	//参数检查
	err := checkSettings(&settings, machineID)
	if err != nil {
		return nil, err
	}

	//参数初始化
	settings.presets = calcPresets(&settings)

	idGen := new(IDGenerator)
	idGen.mutex = new(sync.Mutex)

	idGen.settings = &settings
	idGen.timelineProgress = make([]int64, settings.presets.maxTimeline+1)
	idGen.curTimeline = 0
	idGen.seq = 0
	idGen.machineID = machineID
	return idGen, nil
}

// Generate 生成全局唯一id
func (idGen *IDGenerator) Generate() (int64, error) {
	idGen.mutex.Lock()
	defer idGen.mutex.Unlock()

	settings := idGen.settings
	curTime := idGen.toOffsetTime(time.Now().UnixNano())
	progress := idGen.timelineProgress[idGen.curTimeline] //当前时间线进度

	// 处理时钟回退
	if curTime < progress {
		if curTime < 0 {
			return 0, errors.New("时钟回退时间过长，请检查服务器时钟或设置一个更早的基准时间(Epoch)")
		}

		// 时间小幅回退,等待,直到时间追回
		if progress-curTime < maxWaitTime {
			time.Sleep(time.Millisecond * time.Duration(progress-curTime))
			curTime = idGen.toOffsetTime(time.Now().UnixNano())
		} else {
			//查找合适的时间线
			timeline, err := idGen.findSuitableTimeLine(curTime)
			if err != nil {
				return 0, err
			}

			//切换时间线
			idGen.timelineProgress[idGen.curTimeline] = curTime
			progress = idGen.timelineProgress[timeline]
			idGen.curTimeline = timeline
			idGen.seq = 0
		}
	}

	if curTime == progress {
		//如果当前时间单位的序号已用完，等待直到下一个时间单位
		if idGen.seq = (idGen.seq + 1) & settings.presets.maskSeq; idGen.seq == 0 {
			time.Sleep(time.Duration(idGen.toUnixNano(curTime+1) - time.Now().UnixNano()))
			curTime = idGen.toOffsetTime(time.Now().UnixNano())
		}
	} else {
		idGen.seq = 0
	}

	//时间线向前推进
	idGen.timelineProgress[idGen.curTimeline] = curTime

	if curTime > settings.presets.maxTime {
		return 0, errors.New("当前时间偏移量已超过最大限制，请设置更多的时间位数或设置一个更近的基准时间")
	}

	id := (curTime << settings.presets.shiftTimeBit) |
		(idGen.machineID << settings.presets.shiftMachineIDBit) |
		(idGen.curTimeline << settings.presets.shiftTimelineBit) |
		(idGen.seq)
	return id, nil
}

// findSuitableTimeLine 查找满足当前时间要求的时间线
func (idGen *IDGenerator) findSuitableTimeLine(curTime int64) (int64, error) {
	var fastProgress int64 = -1
	var timeLineFound int64 = -1
	//找出满足当前时间要求且进度最快的时间线
	for index, progress := range idGen.timelineProgress {
		if progress < curTime && progress > fastProgress {
			fastProgress = progress
			timeLineFound = int64(index)
		}
	}
	if timeLineFound == -1 {
		return -1, errors.New("时钟回退太频繁，请调整服务器时钟同步策略或增加时间线数量")
	}
	return timeLineFound, nil
}

func (idGen *IDGenerator) toOffsetTime(unixNano int64) int64 {
	return (unixNano - idGen.settings.Epoch) / int64(timeUnit)
}

func (idGen *IDGenerator) toUnixNano(offset int64) int64 {
	return idGen.settings.Epoch + offset*int64(timeUnit)
}

// Decompose 将id解析成time、seq等部分
func (idGen *IDGenerator) Decompose(id int64) *IDCompose {
	presets := idGen.settings.presets
	time := (int64(id) & presets.maskTime) >> presets.shiftTimeBit
	machineID := (int64(id) & presets.maskMachineID) >> presets.shiftMachineIDBit
	timeline := (int64(id) & presets.maskTimeline) >> presets.shiftTimelineBit
	seq := (int64(id) & presets.maskSeq) >> presets.shiftSeq
	return &IDCompose{
		Time:      time,
		MachineID: machineID,
		TimeLine:  timeline,
		Seq:       seq,
	}
}

// ToReadable 将int64类型的id转换成时间+序号格式，如：2019090419014733273728
func (idGen *IDGenerator) ToReadable(id int64) string {
	presets := idGen.settings.presets

	//时间部分
	timePart := (id & presets.maskTime) >> presets.shiftTimeBit
	genTime := time.Unix(0, idGen.toUnixNano(timePart))

	//剩余部分
	maxInTime := (1 << presets.shiftTimeBit) - 1
	inTimesPart := id & int64(maxInTime)
	inTimeDigit := len(strconv.Itoa(int(maxInTime))) //十进制位数

	format := fmt.Sprintf("%%s%%0.3d%%0.%dd", inTimeDigit)
	return fmt.Sprintf(format, genTime.Format("20060102150405"), genTime.Nanosecond()/int(timeUnit), inTimesPart)
}
