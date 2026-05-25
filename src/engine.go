package main

import "fmt"

type Engine struct {
	bus        *Bus
	routeTable map[string][]Plugin
}

func NewEngine(bus *Bus) *Engine {
	return &Engine{
		bus:        bus,
		routeTable: make(map[string][]Plugin),
	}
}

func (e *Engine) RegisterRoute(receiver string, plugins []Plugin) {
	e.routeTable[receiver] = plugins
}

func (e *Engine) Route(env *Envelop) {
	fmt.Printf("[DEBUG] Engine.Route 收到, receiver=%s, sender=%s\n", env.Receiver, env.Sender)
	if env.TTL <= 0 {
		fmt.Println("[DEBUG] TTL<=0, 丢弃")
		return
	}
	env.TTL--
	orig := env.Receiver
	workflow, ok := e.routeTable[orig]
	if !ok {
		fmt.Printf("[DEBUG] 路由未找到: %s\n", orig)
		return
	}
	if env.Sender == orig {
		fmt.Println("[DEBUG] sender==receiver, 跳过")
		return
	}
	env.Receiver = ""
	fmt.Printf("[DEBUG] Plugin 链长度: %d\n", len(workflow))
	for i, p := range workflow {
		fmt.Printf("[DEBUG] 执行 Plugin %d/%d, receiver=%s\n", i+1, len(workflow), env.Receiver)
		result := p.Execute(env, nil)
		if result == nil {
			fmt.Printf("[DEBUG] Plugin %d 返回 nil, 链终止\n", i+1)
			return
		}
		env = result
		fmt.Printf("[DEBUG] Plugin %d 完成, receiver=%s\n", i+1, env.Receiver)
	}
	env.Sender = orig
	if env.Receiver != "" {
		e.bus.Publish(env.Receiver, env.Clone())
	} else if bt, ok := env.Meta["backtrack_to"].(string); ok && bt != "" {
		e.bus.Publish(bt, env.Clone())
	}
	fmt.Println("[DEBUG] Engine.Route 完成")
}
