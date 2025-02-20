// app interface for graphical user interface.
package app

import (
	"github.com/henrylee2cn/pholcus/crawl"
	"github.com/henrylee2cn/pholcus/crawl/scheduler"
	. "github.com/henrylee2cn/pholcus/node"
	"github.com/henrylee2cn/pholcus/node/task"
	"github.com/henrylee2cn/pholcus/reporter"
	"github.com/henrylee2cn/pholcus/runtime/cache"
	"github.com/henrylee2cn/pholcus/runtime/status"
	"github.com/henrylee2cn/pholcus/spider"
	_ "github.com/henrylee2cn/pholcus/spider/spiders"
	"log"
	"strconv"
	"time"
)

type App interface {
	// 获取全部蜘蛛种类
	GetAllSpiders() []*spider.Spider

	// 通过名字获取某蜘蛛
	GetSpiderByName(string) *spider.Spider

	// 获取执行队列中蜘蛛总数
	SpiderQueueLen() int

	// status.OFFLINE  status.SERVER  status.CLIENT
	SetRunMode(int) App

	// server与client模式下设置
	SetPort(int) App

	// client模式下设置
	SetMaster(string) App

	// 以下Set类方法均为Offline和Server模式用到的
	SetThreadNum(uint) App
	SetBaseSleeptime(uint) App
	SetRandomSleepPeriod(uint) App
	SetOutType(string) App
	SetDockerCap(uint) App
	SetMaxPage(int) App

	// Offline模式下设置
	// SetSpiderQueue()必须在设置全局运行参数之后运行
	// original为spider包中未有过赋值操作的原始蜘蛛种类
	// 已被显式赋值过的spider将不再重新分配Keyword
	SetSpiderQueue(original []spider.Spider, keywords string) App

	// 阻塞等待运行结束
	WaitStop()

	// Offline 模式下中途终止任务
	Stop()

	// server模式下生成任务的方法，必须在全局配置和蜘蛛队列设置完成后才可调用
	CreateTask()

	// 运行前准备
	Ready() App
	// Run()必须在最后运行
	Run()
}

type Logic struct {
	spider.Traversal
	finish chan bool
}

func New() App {
	return &Logic{
		Traversal: spider.Menu,
		finish:    make(chan bool, 1),
	}
}

func (self *Logic) GetAllSpiders() []*spider.Spider {
	return self.Traversal.Get()
}

func (self *Logic) GetSpiderByName(name string) *spider.Spider {
	return self.Traversal.GetByName(name)
}

func (self *Logic) SetRunMode(mode int) App {
	cache.Task.RunMode = mode
	return self
}

func (self *Logic) SetPort(port int) App {
	cache.Task.Port = port
	return self
}

func (self *Logic) SetMaster(master string) App {
	cache.Task.Master = master
	return self
}

func (self *Logic) SetThreadNum(threadNum uint) App {
	cache.Task.ThreadNum = threadNum
	return self
}

func (self *Logic) SetBaseSleeptime(baseSleeptime uint) App {
	cache.Task.BaseSleeptime = baseSleeptime
	return self
}

func (self *Logic) SetRandomSleepPeriod(randomSleepPeriod uint) App {
	cache.Task.RandomSleepPeriod = randomSleepPeriod
	return self
}

func (self *Logic) SetOutType(outType string) App {
	cache.Task.OutType = outType
	return self
}

func (self *Logic) SetDockerCap(dockerCap uint) App {
	cache.Task.DockerCap = dockerCap
	cache.AutoDockerQueueCap()
	return self
}

func (self *Logic) SetMaxPage(maxPage int) App {
	cache.Task.MaxPage = maxPage
	return self
}

// SetSpiderQueue()必须在设置全局运行参数之后运行
// original为spider包中未有过赋值操作的原始蜘蛛种类
// 已被显式赋值过的spider将不再重新分配Keyword
func (self *Logic) SetSpiderQueue(original []spider.Spider, keywords string) App {
	Pholcus.Spiders.Reset()
	// 遍历任务
	for _, sp := range original {
		sp.SetPausetime(cache.Task.BaseSleeptime, cache.Task.RandomSleepPeriod)
		sp.SetMaxPage(cache.Task.MaxPage)
		me := sp
		Pholcus.Spiders.Add(&me)
	}
	// 遍历关键词
	Pholcus.Spiders.AddKeywords(keywords)
	return self
}

func (self *Logic) SpiderQueueLen() int {
	return Pholcus.Spiders.Len()
}

// server模式下分发任务，必须在SpiderQueueL()执行之后调用
func (self *Logic) CreateTask() {
	// 便利添加任务到库
	tasksNum, spidersNum := Pholcus.AddNewTask()

	// 打印报告
	log.Println(` ********************************************************************************************************************************************** `)
	log.Printf(" * ")
	log.Printf(" *                               —— 本次成功添加 %v 条任务，共包含 %v 条采集规则 ——", tasksNum, spidersNum)
	log.Printf(" * ")
	log.Println(` ********************************************************************************************************************************************** `)
}

func (self *Logic) Ready() App {
	// 开启报告
	reporter.Log.Run()

	// 运行pholcus核心
	PholcusRun()
	return self
}

func (self *Logic) Run() {
	switch cache.Task.RunMode {
	case status.OFFLINE:
		self.offline()
	case status.SERVER:
		self.server()
	case status.CLIENT:
		self.client()
	default:
		log.Println(" *    ——请指定正确的运行模式！——")
		return
	}
}

// Offline 模式下中途终止任务
func (self *Logic) Stop() {
	status.Crawl = status.STOP
	Pholcus.Crawls.Stop()
	scheduler.Sdl.Stop()
	reporter.Log.Stop()

	// 总耗时
	takeTime := time.Since(cache.StartTime).Minutes()

	// 打印总结报告
	log.Println(` ********************************************************************************************************************************************** `)
	log.Printf(" * ")
	log.Printf(" *                               ！！任务取消：下载页面 %v 个，耗时：%.5f 分钟！！", cache.ReqSum, takeTime)
	log.Printf(" * ")
	log.Println(` ********************************************************************************************************************************************** `)

	// 标记结束
	self.finish <- true
}

// 阻塞等待运行结束
func (self *Logic) WaitStop() {
	<-self.finish
}

// ******************************************** 私有方法 ************************************************* \\

func (self *Logic) offline() {
	// 每次执行重新开启报告
	reporter.Log.Run()
	self.exec()
}

func (self *Logic) server() {}

func (self *Logic) client() {
	for {
		// reporter.Log.Println("开始获取任务")

		// 从任务库获取一个任务
		t := Pholcus.DownTask()
		// reporter.Log.Printf("成功获取任务 %#v", t)

		// 准备运行
		self.taskToRun(t)

		// 执行任务
		self.exec()

		self.WaitStop()
	}
}

// client模式下从task准备运行条件
func (self *Logic) taskToRun(t *task.Task) {
	// 清空历史任务
	Pholcus.Spiders.Reset()

	// 更改全局配置
	cache.Task.OutType = t.OutType
	cache.Task.ThreadNum = t.ThreadNum
	cache.Task.DockerCap = t.DockerCap
	cache.Task.DockerQueueCap = t.DockerQueueCap

	// 初始化蜘蛛队列
	for _, n := range t.Spiders {
		if sp := spider.Menu.GetByName(n["name"]); sp != nil {
			sp.SetPausetime(t.BaseSleeptime, t.RandomSleepPeriod)
			sp.SetMaxPage(t.MaxPage)
			if v, ok := n["keyword"]; ok {
				sp.SetKeyword(v)
			}
			one := *sp
			Pholcus.Spiders.Add(&one)
		}
	}
}

// 开始执行任务
func (self *Logic) exec() {
	count := Pholcus.Spiders.Len()
	cache.ReqSum = 0

	// 初始化资源队列
	scheduler.Init(cache.Task.ThreadNum)

	// 设置爬虫队列
	crawlNum := Pholcus.Crawls.Reset(count)

	log.Println(` ********************************************************************************************************************************************** `)
	log.Printf(" * ")
	log.Printf(" *     执行任务总数（任务数[*关键词数]）为 %v 个...\n", count)
	log.Printf(" *     爬虫队列可容纳蜘蛛 %v 只...\n", crawlNum)
	log.Printf(" *     并发协程最多 %v 个……\n", cache.Task.ThreadNum)
	log.Printf(" *     随机停顿时间为 %v~%v ms ……\n", cache.Task.BaseSleeptime, cache.Task.BaseSleeptime+cache.Task.RandomSleepPeriod)
	log.Printf(" * ")
	log.Printf(" *                                                                                                             —— 开始抓取，请耐心等候 ——")
	log.Printf(" * ")
	log.Println(` ********************************************************************************************************************************************** `)

	// 开始计时
	cache.StartTime = time.Now()

	// 任务执行
	status.Crawl = status.RUN

	// 根据模式选择合理的并发
	if cache.Task.RunMode == status.OFFLINE {
		go self.goRun(count)
	} else {
		// 保证了打印信息的同步输出
		self.goRun(count)
	}
}

// 任务执行
func (self *Logic) goRun(count int) {
	for i := 0; i < count && status.Crawl == status.RUN; i++ {
		// 从爬行队列取出空闲蜘蛛，并发执行
		c := Pholcus.Crawls.Use()
		if c != nil {
			go func(i int, c crawl.Crawler) {
				// 执行并返回结果消息
				c.Init(Pholcus.Spiders.GetByIndex(i)).Start()
				// 任务结束后回收该蜘蛛
				Pholcus.Crawls.Free(c.GetId())
			}(i, c)
		}
	}

	// 监控结束任务
	sum := 0 //数据总数
	for i := 0; i < count; i++ {
		s := <-cache.ReportChan

		log.Println(` ********************************************************************************************************************************************** `)
		log.Printf(" * ")
		reporter.Log.Printf(" *     [结束报告 -> 任务：%v | 关键词：%v]   共输出数据 %v 条，用时 %v 分钟！\n", s.SpiderName, s.Keyword, s.Num, s.Time)
		log.Printf(" * ")
		log.Println(` ********************************************************************************************************************************************** `)

		if slen, err := strconv.Atoi(s.Num); err == nil {
			sum += slen
		}
	}

	// 总耗时
	takeTime := time.Since(cache.StartTime).Minutes()

	// 打印总结报告
	log.Println(` ********************************************************************************************************************************************** `)
	log.Printf(" * ")
	reporter.Log.Printf(" *                               —— 本次抓取合计 %v 条数据，下载页面 %v 个，耗时：%.5f 分钟 ——", sum, cache.ReqSum, takeTime)
	log.Printf(" * ")
	log.Println(` ********************************************************************************************************************************************** `)

	// 标记结束
	self.finish <- true
}
