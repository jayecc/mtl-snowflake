# mtl-snowflake
*multi-timeline-snowflake 基于多时间线改进的snowflake分布式id生成器*  

  - snowflake(雪花)算法是twitter开源的分布式id生成算法，主要优点有：
    - 极少依赖(仅依赖系统时钟)
    - 分布式全局唯一
    - 满足趋势递增
    - 理论上限(推荐设置)：单机每秒可生成409.6万ID(集群42亿/s)
```text
                          snowflake 推荐id结构

  /-- 符号位 --\ /-------时间戳--------\ /--机器id- -\ /---序列号----\
 |      0       |------   41bit   ------|--  10bit  --|---  12bit  ---|

                           mtl-snowflake 推荐id结构

  /-- 符号位 --\ /-------时间戳--------\ /--机器id- -\ /---时间线--\ /---序列号----\
 |      0       |------   41bit   ------|--   9bit  --|---  1bit  --|---  12bit  ---|
```
 mtl-snowflake在snowflake id结构中增加时间线部分(推荐1位)，通过设置多条时间线，来解决当发生时钟回退时的id重复问题。
   - step1 初始时，算法随机选定一条时间线作为当前时间线，生成id的同时推进当前时间线进度。
   - step2 当发生时钟回退，算法暂停当前时间线进度，选择一条合适的时间线(进度<当前时间)并切换到该时间线，这样算法可以继续生成不重复的ID

 **mtl-snowflake：**
   - 时钟回退情况下仍能保证id全局唯一
   - 理论上限(推荐设置)：单机每秒可生成409.6万ID(集群21亿/s)

# 关于参数设置

**- 机器ID(machineID)**  
  
  - 机器ID又称节点ID,用于区分同一集群的不同节点，确保集群内ID的全局唯一性。    
    - 对同业务内不同实例：必须保证节点ID不同，否则不能保证所生成id的全局唯一性。  
    - 对不同业务间实例：通常不同业务ID互不干扰，允许id重复，所有不同业务之间允许节点ID相同。 
 
**- 自定义参数**  

  - mtl-snowflake支持业务根据各自各需要调整相应参数，但同一业务必须指定相同的参数(除机器ID)，否则不能保证生成的ID是全局唯一的。
  
# 如何使用
```go
	//使用默认配置
	// - TimeBit=41 可使用64年
	// - MachineIDBit=9 最多512个节点
	// - TimelineBit=1  双时间线
	// - SeqBit=12 1毫秒内最多生成4096个序号
	// - Epoch=1433865600000000000(2015.6.10 00:00:00) 基准时间(unix nano)
	machineID := 0 //节点id
	idGen, err := NewGenerator(machineID)
	if err != nil {
		//panic(err)
	}

	// 数字形式：560780571450613760
	id, err := idGen.Generate()
	if err != nil {
		//panic(err)
	}

	// 时间+序号：2019090419014733273728
	readableID := idGen.ToReadable(id)

	//如果是集群,所有节点使用的配置必须一致，但需指定不同的machineID
	machineID := 1 //节点id
	idGen, err := NewGenerator(machineID)

```

## 使用自定义配置
 - 可以根据自身业务特点调整配置，比如业务集群的节点较少，但单机吞吐量要求较高，可适当减少MachineID位数，并增加SeqBit位数
```go
	// 最多64节点
	machineID := 0
	idGen, err :=NewGeneratorWithSettings(machineID, Settings{
		TimeBit:     41,
		MachineIDBit: 6,
		TimelineBit: 1,
		SeqBit:      15,
		Epoch:       DefaultEpoch,
	})
	id, err := idGen.Generate()
	
	// 单时间线，当发生时钟回退时将返回error
	machineID := 0
	idGen, err :=NewGeneratorWithSettings(machineID, Settings{
		TimeBit:     41,
		MachineIDBit: 10,
		TimelineBit: 0,
		SeqBit:      12,
		Epoch:       DefaultEpoch,
	})
	id, err := idGen.Generate()
	if err != nil {
		//panic(err)
	}
```