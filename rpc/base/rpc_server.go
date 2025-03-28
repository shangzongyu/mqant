// Copyright 2014 mqant Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package defaultrpc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/shangzongyu/mqant/log"
	"github.com/shangzongyu/mqant/module"
	mqrpc "github.com/shangzongyu/mqant/rpc"
	rpcpb "github.com/shangzongyu/mqant/rpc/pb"
	argsutil "github.com/shangzongyu/mqant/rpc/util"
	"google.golang.org/protobuf/proto"
)

type RPCServer struct {
	module       module.Module
	app          module.App
	functions    map[string]*mqrpc.FunctionInfo
	natsServer   *NatsServer
	mqChan       chan mqrpc.CallInfo // 接收到请求信息的队列
	wg           sync.WaitGroup      // 任务阻塞
	callChanDone chan error
	listener     mqrpc.RPCListener
	control      mqrpc.GoroutineControl // 控制模块可同时开启的最大协程数
	executing    int64                  // 正在执行的goroutine数量
}

func NewRPCServer(app module.App, module module.Module) (mqrpc.RPCServer, error) {
	rpcServer := new(RPCServer)
	rpcServer.app = app
	rpcServer.module = module
	rpcServer.callChanDone = make(chan error)
	rpcServer.functions = make(map[string]*mqrpc.FunctionInfo)
	rpcServer.mqChan = make(chan mqrpc.CallInfo)

	natsServer, err := NewNatsServer(app, rpcServer)
	if err != nil {
		log.Error("AMQPServer Dial: %s", err)
	}
	rpcServer.natsServer = natsServer

	// go rpc_server.on_call_handle(rpc_server.mq_chan, rpc_server.call_chan_done)

	return rpcServer, nil
}

func (rpcs *RPCServer) Addr() string {
	return rpcs.natsServer.Addr()
}

func (rpcs *RPCServer) SetListener(listener mqrpc.RPCListener) {
	rpcs.listener = listener
}

func (rpcs *RPCServer) SetGoroutineControl(control mqrpc.GoroutineControl) {
	rpcs.control = control
}

// GetExecuting 获取当前正在执行的goroutine 数量
func (rpcs *RPCServer) GetExecuting() int64 {
	return rpcs.executing
}

// Register you must call the function before calling Open and Go
func (rpcs *RPCServer) Register(id string, f interface{}) {
	if _, ok := rpcs.functions[id]; ok {
		panic(fmt.Sprintf("function id %v: already registered", id))
	}
	finfo := &mqrpc.FunctionInfo{
		Function:  reflect.ValueOf(f),
		FuncType:  reflect.ValueOf(f).Type(),
		Goroutine: false,
	}

	finfo.InType = []reflect.Type{}
	for i := 0; i < finfo.FuncType.NumIn(); i++ {
		rv := finfo.FuncType.In(i)
		finfo.InType = append(finfo.InType, rv)
	}
	rpcs.functions[id] = finfo
}

// RegisterGO you must call the function before calling Open and Go
func (rpcs *RPCServer) RegisterGO(id string, f interface{}) {
	if _, ok := rpcs.functions[id]; ok {
		panic(fmt.Sprintf("function id %v: already registered", id))
	}

	finfo := &mqrpc.FunctionInfo{
		Function:  reflect.ValueOf(f),
		FuncType:  reflect.ValueOf(f).Type(),
		Goroutine: true,
	}

	finfo.InType = []reflect.Type{}
	for i := 0; i < finfo.FuncType.NumIn(); i++ {
		rv := finfo.FuncType.In(i)
		finfo.InType = append(finfo.InType, rv)
	}
	rpcs.functions[id] = finfo
}

func (rpcs *RPCServer) Done() (err error) {
	//等待正在执行的请求完成
	//close(s.mq_chan)   //关闭mq_chan通道
	//<-s.call_chan_done //mq_chan通道的信息都已处理完
	rpcs.wg.Wait()
	// s.call_chan_done <- nil
	// 关闭队列链接
	if rpcs.natsServer != nil {
		err = rpcs.natsServer.Shutdown()
	}
	return
}

func (rpcs *RPCServer) Call(callInfo *mqrpc.CallInfo) error {
	rpcs.runFunc(callInfo)
	//if callInfo.RPCInfo.Expired < (time.Now().UnixNano() / 1000000) {
	//	//请求超时了,无需再处理
	//	if s.listener != nil {
	//		s.listener.OnTimeOut(callInfo.RPCInfo.Fn, callInfo.RPCInfo.Expired)
	//	} else {
	//		log.Warning("timeout: This is Call", s.module.GetType(), callInfo.RPCInfo.Fn, callInfo.RPCInfo.Expired, time.Now().UnixNano()/1000000)
	//	}
	//} else {
	//	s.runFunc(callInfo)
	//	//go func() {
	//	//	resultInfo := rpcpb.NewResultInfo(callInfo.RPCInfo.Cid, "", argsutil.STRING, []byte("success"))
	//	//	callInfo.Result = *resultInfo
	//	//	s.doCallback(callInfo)
	//	//}()
	//
	//}
	return nil
}

func (rpcs *RPCServer) doCallback(callInfo *mqrpc.CallInfo) {
	if callInfo.RPCInfo.Reply {
		// 需要回复的才回复
		err := callInfo.Agent.(mqrpc.MQServer).Callback(callInfo)
		if err != nil {
			log.Warning("rpc callback erro :\n%s", err.Error())
		}

		//if callInfo.RPCInfo.Expired < (time.Now().UnixNano() / 1000000) {
		//	//请求超时了,无需再处理
		//	err := callInfo.Agent.(mqrpc.MQServer).Callback(callInfo)
		//	if err != nil {
		//		log.Warning("rpc callback erro :\n%s", err.Error())
		//	}
		//}else {
		//	log.Warning("timeout: This is Call %s %s", s.module.GetType(), callInfo.RPCInfo.Fn)
		//}
	} else {
		// 对于不需要回复的消息,可以判断一下是否出现错误，打印一些警告
		if callInfo.Result.Error != "" {
			log.Warning("rpc callback erro :\n%s", callInfo.Result.Error)
		}
	}
	if rpcs.app.Options().ServerRPCHandler != nil {
		rpcs.app.Options().ServerRPCHandler(rpcs.app, rpcs.module, callInfo)
	}
}

func (rpcs *RPCServer) _errorCallback(start time.Time, callInfo *mqrpc.CallInfo, Cid string, Error string) {
	// 异常日志都应该打印
	// log.TError(span, "rpc Exec ModuleType = %v Func = %v Elapsed = %v ERROR:\n%v", s.module.GetType(), callInfo.RPCInfo.Fn, time.Since(start), Error)
	resultInfo := rpcpb.NewResultInfo(Cid, Error, argsutil.NULL, nil)
	callInfo.Result = resultInfo
	callInfo.ExecTime = time.Since(start).Nanoseconds()
	rpcs.doCallback(callInfo)
	if rpcs.listener != nil {
		rpcs.listener.OnError(callInfo.RPCInfo.Fn, callInfo, fmt.Errorf(Error))
	}
}

func (rpcs *RPCServer) _runFunc(start time.Time, functionInfo *mqrpc.FunctionInfo, callInfo *mqrpc.CallInfo) {
	f := functionInfo.Function
	fType := functionInfo.FuncType
	fInType := functionInfo.InType
	params := callInfo.RPCInfo.Args
	ArgsType := callInfo.RPCInfo.ArgsType
	if len(params) != fType.NumIn() {
		// 因为在调研的 _func的时候还会额外传递一个回调函数 cb
		rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, fmt.Sprintf("The number of params %v is not adapted.%v", params, f.String()))
		return
	}

	rpcs.wg.Add(1)
	rpcs.executing++
	defer func() {
		rpcs.wg.Add(-1)
		rpcs.executing--
		if rpcs.control != nil {
			rpcs.control.Finish()
		}
		if r := recover(); r != nil {
			rn := ""
			switch r.(type) {

			case string:
				rn = r.(string)
			case error:
				rn = r.(error).Error()
			}
			buf := make([]byte, 1024)
			l := runtime.Stack(buf, false)
			errStr := string(buf[:l])
			allError := fmt.Sprintf("%s rpc func(%s) error %s\n ----Stack----\n%s", rpcs.module.GetType(), callInfo.RPCInfo.Fn, rn, errStr)
			log.Error(allError)
			rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, allError)
		}
	}()

	// t:=RandInt64(2,3)
	// time.Sleep(time.Second*time.Duration(t))
	// f 为函数地址
	var in []reflect.Value
	var input []interface{}
	if len(ArgsType) > 0 {
		in = make([]reflect.Value, len(params))
		input = make([]interface{}, len(params))
		for k, v := range ArgsType {
			rv := fInType[k]
			var elemp reflect.Value
			if rv.Kind() == reflect.Ptr {
				// 如果是指针类型就得取到指针所代表的具体类型
				elemp = reflect.New(rv.Elem())
			} else {
				elemp = reflect.New(rv)
			}

			if pb, ok := elemp.Interface().(mqrpc.Marshaler); ok {
				err := pb.Unmarshal(params[k])
				if err != nil {
					rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, err.Error())
					return
				}
				if pb == nil { // 多选语句switch
					in[k] = reflect.Zero(rv)
				}
				if rv.Kind() == reflect.Ptr {
					// 接收指针变量的参数
					in[k] = reflect.ValueOf(elemp.Interface())
				} else {
					// 接收值变量
					in[k] = elemp.Elem()
				}
				input[k] = pb
			} else if pb, ok := elemp.Interface().(proto.Message); ok {
				err := proto.Unmarshal(params[k], pb)
				if err != nil {
					rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, err.Error())
					return
				}
				if pb == nil { // 多选语句switch
					in[k] = reflect.Zero(rv)
				}
				if rv.Kind() == reflect.Ptr {
					// 接收指针变量的参数
					in[k] = reflect.ValueOf(elemp.Interface())
				} else {
					// 接收值变量
					in[k] = elemp.Elem()
				}
				input[k] = pb
			} else {
				// 不是 Marshaler 才尝试用 argsutil 解析
				ty, err := argsutil.Bytes2Args(rpcs.app, v, params[k])
				if err != nil {
					rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, err.Error())
					return
				}
				switch v2 := ty.(type) { // 多选语句switch
				case nil:
					in[k] = reflect.Zero(rv)
				case []uint8:
					if reflect.TypeOf(ty).AssignableTo(rv) {
						// 如果ty "继承" 于接受参数类型
						in[k] = reflect.ValueOf(ty)
					} else {
						elemp := reflect.New(rv)
						if err := json.Unmarshal(v2, elemp.Interface()); err != nil {
							log.Error("%v []uint8--> %v error with='%v'", callInfo.RPCInfo.Fn, rv, err)
							in[k] = reflect.ValueOf(ty)
						} else {
							in[k] = elemp.Elem()
						}
					}
				default:
					in[k] = reflect.ValueOf(ty)
				}
				input[k] = in[k].Interface()
			}
		}
	}

	if rpcs.listener != nil {
		errs := rpcs.listener.BeforeHandle(callInfo.RPCInfo.Fn, callInfo)
		if errs != nil {
			rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, errs.Error())
			return
		}
	}

	out := f.Call(in)
	var rs []interface{}
	if len(out) != 2 {
		rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, fmt.Sprintf("%s rpc func(%s) return error %s\n", rpcs.module.GetType(), callInfo.RPCInfo.Fn, "func(....)(result interface{}, err error)"))
		return
	}
	if len(out) > 0 { // prepare out paras
		rs = make([]interface{}, len(out), len(out))
		for i, v := range out {
			rs[i] = v.Interface()
		}
	}
	if rpcs.app.Options().RpcCompleteHandler != nil {
		rpcs.app.Options().RpcCompleteHandler(rpcs.app, rpcs.module, callInfo, input, rs, time.Since(start))
	}
	var rErr string
	switch e := rs[1].(type) {
	case string:
		rErr = e
		break
	case error:
		rErr = e.Error()
	case nil:
		rErr = ""
	default:
		rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, fmt.Sprintf("%s rpc func(%s) return error %s\n", rpcs.module.GetType(), callInfo.RPCInfo.Fn, "func(....)(result interface{}, err error)"))
		return
	}
	argsType, args, err := argsutil.ArgsTypeAnd2Bytes(rpcs.app, rs[0])
	if err != nil {
		rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, err.Error())
		return
	}
	resultInfo := rpcpb.NewResultInfo(
		callInfo.RPCInfo.Cid,
		rErr,
		argsType,
		args,
	)
	callInfo.Result = resultInfo
	callInfo.ExecTime = time.Since(start).Nanoseconds()
	rpcs.doCallback(callInfo)
	if rpcs.app.GetSettings().RPC.Log {
		log.TInfo(nil, "rpc Exec ModuleType = %v Func = %v Elapsed = %v", rpcs.module.GetType(), callInfo.RPCInfo.Fn, time.Since(start))
	}
	if rpcs.listener != nil {
		rpcs.listener.OnComplete(callInfo.RPCInfo.Fn, callInfo, resultInfo, time.Since(start).Nanoseconds())
	}
}

// ---------------------------------if _func is not a function or para num and type not match,it will cause panic
func (rpcs *RPCServer) runFunc(callInfo *mqrpc.CallInfo) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			rn := ""
			switch r.(type) {

			case string:
				rn = r.(string)
			case error:
				rn = r.(error).Error()
			}
			log.Error("recover", rn)
			rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, rn)
		}
	}()

	if rpcs.control != nil {
		// 协程数量达到最大限制
		if err := rpcs.control.Wait(); err != nil {
			// TODO: add log ro something else
			return
		}
	}
	functionInfo, ok := rpcs.functions[callInfo.RPCInfo.Fn]
	if !ok {
		if rpcs.listener != nil {
			fInfo, err := rpcs.listener.NoFoundFunction(callInfo.RPCInfo.Fn)
			if err != nil {
				rpcs._errorCallback(start, callInfo, callInfo.RPCInfo.Cid, err.Error())
				return
			}
			functionInfo = fInfo
		}
	}
	if functionInfo.Goroutine {
		go rpcs._runFunc(start, functionInfo, callInfo)
	} else {
		rpcs._runFunc(start, functionInfo, callInfo)
	}
}
