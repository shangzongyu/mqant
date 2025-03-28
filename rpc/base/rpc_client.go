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
    "context"
    "fmt"
    "os"
    "time"

    "github.com/shangzongyu/mqant/log"
    "github.com/shangzongyu/mqant/module"
    mqrpc "github.com/shangzongyu/mqant/rpc"
    rpcpb "github.com/shangzongyu/mqant/rpc/pb"
    argsutil "github.com/shangzongyu/mqant/rpc/util"
    "github.com/shangzongyu/mqant/utils/uuid"
    "google.golang.org/protobuf/proto"
)

type RPCClient struct {
    app        module.App
    natsClient *NatsClient
}

func NewRPCClient(app module.App, session module.ServerSession) (mqrpc.RPCClient, error) {
    rpcClient := new(RPCClient)
    rpcClient.app = app
    natsClient, err := NewNatsClient(app, session)
    if err != nil {
        log.Error("Dial: %s", err)
        return nil, err
    }
    rpcClient.natsClient = natsClient
    return rpcClient, nil
}

func (c *RPCClient) Done() (err error) {
    if c.natsClient != nil {
        err = c.natsClient.Done()
    }
    return
}

func (c *RPCClient) CallArgs(ctx context.Context, _func string, ArgsType []string, args [][]byte) (r interface{}, e string) {
    caller, _ := os.Hostname()
    if ctx != nil {
        cr, ok := ctx.Value("caller").(string)
        if ok {
            caller = cr
        }
    }
    start := time.Now()
    correlationId := uuid.Rand().Hex()
    rpcInfo := &rpcpb.RPCInfo{
        Fn:       *proto.String(_func),
        Reply:    *proto.Bool(true),
        Expired:  *proto.Int64((start.UTC().Add(c.app.Options().RPCExpired).UnixNano()) / 1000000),
        Cid:      *proto.String(correlationId),
        Args:     args,
        ArgsType: ArgsType,
        Caller:   *proto.String(caller),
        Hostname: *proto.String(caller),
    }
    defer func() {
        // 异常日志都应该打印
        if c.app.Options().ClientRPChandler != nil {
            exec_time := time.Since(start).Nanoseconds()
            c.app.Options().ClientRPChandler(c.app, *c.natsClient.session.GetNode(), rpcInfo, r, e, exec_time)
        }
    }()
    callInfo := &mqrpc.CallInfo{
        RPCInfo: rpcInfo,
    }
    callback := make(chan *rpcpb.ResultInfo, 1)
    var err error
    //优先使用本地rpc
    //if c.local_client != nil {
    //	err = c.local_client.Call(*callInfo, callback)
    //} else
    err = c.natsClient.Call(callInfo, callback)
    if err != nil {
        return nil, err.Error()
    }
    if ctx == nil {
        ctx, _ = context.WithTimeout(context.TODO(), c.app.Options().RPCExpired)
    }
    select {
    case resultInfo, ok := <-callback:
        if !ok {
            return nil, "client closed"
        }
        result, err := argsutil.Bytes2Args(c.app, resultInfo.ResultType, resultInfo.Result)
        if err != nil {
            return nil, err.Error()
        }
        return result, resultInfo.Error
    case <-ctx.Done():
        _ = c.natsClient.Delete(rpcInfo.Cid)
        c.closeCallbackChan(callback)
        return nil, "deadline exceeded"
        // case <-time.After(time.Second * time.Duration(c.app.GetSettings().rpc.RPCExpired)):
        //	close(callback)
        //	c.nats_client.Delete(rpcInfo.Cid)
        //	return nil, "deadline exceeded"
    }
}

func (c *RPCClient) closeCallbackChan(ch chan *rpcpb.ResultInfo) {
    defer func() {
        if recover() != nil {
            // close(ch) panic occur
        }
    }()

    close(ch) // panic if ch is closed
}

func (c *RPCClient) CallNRArgs(_func string, argsType []string, args [][]byte) (err error) {
    caller, _ := os.Hostname()
    correlationId := uuid.Rand().Hex()
    rpcInfo := &rpcpb.RPCInfo{
        Fn:       *proto.String(_func),
        Reply:    *proto.Bool(false),
        Expired:  *proto.Int64((time.Now().UTC().Add(c.app.Options().RPCExpired).UnixNano()) / 1000000),
        Cid:      *proto.String(correlationId),
        Args:     args,
        ArgsType: argsType,
        Caller:   *proto.String(caller),
        Hostname: *proto.String(caller),
    }
    callInfo := &mqrpc.CallInfo{
        RPCInfo: rpcInfo,
    }
    //优先使用本地rpc
    //if c.local_client != nil {
    //	err = c.local_client.CallNR(*callInfo)
    //} else
    return c.natsClient.CallNR(callInfo)
}

// Call 消息请求 需要回复
func (c *RPCClient) Call(ctx context.Context, _func string, params ...interface{}) (interface{}, string) {
    var ArgsType []string = make([]string, len(params))
    var args [][]byte = make([][]byte, len(params))
    var span log.TraceSpan = nil
    for k, param := range params {
        var err error = nil
        ArgsType[k], args[k], err = argsutil.ArgsTypeAnd2Bytes(c.app, param)
        if err != nil {
            return nil, fmt.Sprintf("args[%d] error %s", k, err.Error())
        }
        switch v2 := param.(type) { // 多选语句switch
        case log.TraceSpan:
            // 如果参数是这个需要拷贝一份新的再传
            span = v2
        }
    }
    start := time.Now()
    r, errstr := c.CallArgs(ctx, _func, ArgsType, args)
    if c.app.GetSettings().RPC.Log {
        log.TInfo(span, "rpc Call ServerId = %v Func = %v Elapsed = %v Result = %v ERROR = %v", c.natsClient.session.GetID(), _func, time.Since(start), r, errstr)
    }
    return r, errstr
}

// CallNR 消息请求 不需要回复
func (c *RPCClient) CallNR(_func string, params ...interface{}) (err error) {
    var ArgsType []string = make([]string, len(params))
    var args [][]byte = make([][]byte, len(params))
    var span log.TraceSpan = nil
    for k, param := range params {
        ArgsType[k], args[k], err = argsutil.ArgsTypeAnd2Bytes(c.app, param)
        if err != nil {
            return fmt.Errorf("args[%d] error %s", k, err.Error())
        }

        switch v2 := param.(type) { // 多选语句switch
        case log.TraceSpan:
            span = v2
        }
    }
    start := time.Now()
    err = c.CallNRArgs(_func, ArgsType, args)
    if c.app.GetSettings().RPC.Log {
        log.TInfo(span, "rpc CallNR ServerId = %v Func = %v Elapsed = %v ERROR = %v", c.natsClient.session.GetID(), _func, time.Since(start), err)
    }
    return err
}
