package generator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestNewGeneratorWithSettings 创建生成器
func TestNewGeneratorWithSettings(t *testing.T) {
	type Args struct {
		Settings  Settings
		MachineID int64
	}
	testCases := []struct {
		name string
		args Args
		want bool
	}{
		{name: "各部分位数和校验成功", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 10, TimelineBit: 0, SeqBit: 12, Epoch: DefaultEpoch}, MachineID: 0}, want: true},
		{name: "各部分位数和校验成功", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, MachineID: 0}, want: true},
		{name: "各部分位数和校验失败", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 10, TimelineBit: 0, SeqBit: 11, Epoch: DefaultEpoch}, MachineID: 0}, want: false},
		{name: "各部分位数和校验失败", args: Args{Settings: Settings{TimeBit: 42, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, MachineID: 0}, want: false},
		{name: "基准时间过晚校验失败", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: time.Now().Add(time.Second).UnixNano()}, MachineID: 0}, want: false},
		{name: "时间位数太少校验失败", args: Args{Settings: Settings{TimeBit: 35, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: time.Now().AddDate(-3, 0, 0).UnixNano()}, MachineID: 0}, want: false},
		{name: "machineIDBit为0校验成功", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 0, TimelineBit: 1, SeqBit: 21, Epoch: DefaultEpoch}, MachineID: 0}, want: true},
		{name: "machineID超限校验失败", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, MachineID: -1}, want: false},
		{name: "machineID超限校验失败", args: Args{Settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, MachineID: 1024}, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewGeneratorWithSettings(tc.args.MachineID, tc.args.Settings)
			got := err == nil
			if got != tc.want {
				bytes, _ := json.Marshal(tc.args)
				t.Fatalf("【失败】-%s-got:%v-want:%v-参数：%s", tc.name, got, tc.want, string(bytes))
			}
		})
	}
}

// TestUniqueID id全局唯一
func TestUniqueID(t *testing.T) {
	var ids sync.Map

	for machineID := int64(0); machineID < 10; machineID++ {
		func(machineID int64) {
			t.Run(strconv.Itoa(int(machineID)), func(t *testing.T) {
				// 并行测试
				t.Parallel()

				idGen, err := NewGenerator(machineID)
				if err != nil {
					t.Fatal(err.Error())
				}

				//生成十万个id,判断是否有重复
				for i := 1e6; i > 0; i-- {
					id, err := idGen.Generate()
					if err != nil {
						t.Fatal(err.Error())
					}
					if _, exist := ids.Load(int64(id)); exist {
						t.Fatalf("出现重复的id:%d", int64(id))
					}
					ids.Store(int64(id), nil)
				}
			})
		}(machineID)
	}
}

// TestTimeBackward 时钟回退
// - 通过调整基准时间，模拟时钟回退
func TestTimeBackward(t *testing.T) {
	testCases := []struct {
		name      string
		settings  Settings
		backCount int
		want      bool //是否生成成功
	}{
		{name: "1时间线0次时钟回退检验成功", settings: Settings{TimeBit: 41, MachineIDBit: 10, TimelineBit: 0, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 0, want: true},
		{name: "1时间线1次时钟回退检验失败", settings: Settings{TimeBit: 41, MachineIDBit: 10, TimelineBit: 0, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 1, want: false},
		{name: "2时间线1次时钟回退检验成功", settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 1, want: true},
		{name: "2时间线2次时钟回退检验失败", settings: Settings{TimeBit: 41, MachineIDBit: 9, TimelineBit: 1, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 2, want: false},
		{name: "4时间线3次时钟回退检验成功", settings: Settings{TimeBit: 41, MachineIDBit: 8, TimelineBit: 2, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 3, want: true},
		{name: "4时间线4次时钟回退检验失败", settings: Settings{TimeBit: 41, MachineIDBit: 8, TimelineBit: 2, SeqBit: 12, Epoch: DefaultEpoch}, backCount: 4, want: false},
	}

	generator := func(idGen *IDGenerator, ids map[int64]interface{}, count int64) error {
		for i := count; i > 0; i-- {
			id, err := idGen.Generate()
			if err != nil {
				return err
			}
			if _, exist := ids[int64(id)]; exist {
				return errors.New(fmt.Sprintf("出现重复的id:%d", int64(id)))
			}
			ids[int64(id)] = nil
		}
		return nil
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idGen, _ := NewGeneratorWithSettings(0, tc.settings)
			ids := make(map[int64]interface{})

			// 记录开始时间点
			startTime := time.Now().UnixNano()
			// step 1 先生成一批id
			err := generator(idGen, ids, 1e7)

			if err != nil {
				t.Fatalf("【失败】-%s-want:%v-got:%v", tc.name, true, err == nil)
			}

			for backCount := 0; backCount < tc.backCount; backCount++ {
				// step 2 回退到开始时间点
				curTime := time.Now().UnixNano()
				idGen.settings.Epoch += curTime - startTime

				// step 3 继续生成
				err := generator(idGen, ids, 1e7)
				got := err == nil
				want := tc.want

				// 前tc.backCount-1次回退必须成功
				if backCount < tc.backCount-1 {
					want = true
				}

				if got != want {
					t.Fatalf("【失败】-%s-want:%v-got:%v", tc.name, want, got)
				}
			}
		})
	}
}

// TestDecompose ID解构
func TestDecompose(t *testing.T) {
	type TestCaseEntity struct {
		name  string
		idGen *IDGenerator
		id    int64
		want  IDCompose
	}

	idGen0, _ := NewGenerator(0)
	idGen1, _ := NewGenerator(1)

	id0, _ := idGen0.Generate()
	id1, _ := idGen0.Generate()
	id2, _ := idGen1.Generate()
	id3, _ := idGen1.Generate()

	testCases := []TestCaseEntity{
		{name: "验证成功case1", idGen: idGen0, id: id0, want: IDCompose{MachineID: 0, Seq: 0}},
		{name: "验证成功case2", idGen: idGen0, id: id1, want: IDCompose{MachineID: 0, Seq: 1}},
		{name: "验证成功case3", idGen: idGen1, id: id2, want: IDCompose{MachineID: 1, Seq: 0}},
		{name: "验证成功case4", idGen: idGen1, id: id3, want: IDCompose{MachineID: 1, Seq: 1}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.idGen.Decompose(tc.id)
			if got.MachineID != tc.want.MachineID ||
				got.Seq != tc.want.Seq {
				t.Fatalf("【失败】-%s-got:%v,want:%v", tc.name, got, tc.want)
			}
		})
	}

}

// BenchmarkGenSeqBit12 单节点(12位序列号)性能测试
func BenchmarkGenSeqBit12(b *testing.B) {
	idGen, _ := NewGenerator(0)
	for i := 0; i < b.N; i++ {
		idGen.Generate()
	}
}

// BenchmarkGenSeqBit14 单节点(14位序列号)性能测试
func BenchmarkGenSeqBit14(b *testing.B) {
	idGen, _ := NewGeneratorWithSettings(0, Settings{
		TimeBit:      41,
		MachineIDBit: 7,
		TimelineBit:  1,
		SeqBit:       14,
		Epoch:        DefaultEpoch,
	})
	for i := 0; i < b.N; i++ {
		idGen.Generate()
	}
}

// BenchmarkGenSeqBit21 单节点21位序列号)性能测试
func BenchmarkGenSeqBit21(b *testing.B) {
	idGen, _ := NewGeneratorWithSettings(0, Settings{
		TimeBit:      41,
		MachineIDBit: 0,
		TimelineBit:  1,
		SeqBit:       21,
		Epoch:        DefaultEpoch,
	})
	for i := 0; i < b.N; i++ {
		idGen.Generate()
	}
}
