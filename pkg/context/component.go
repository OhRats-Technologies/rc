package context

type ProvidedService struct {
	Name  string
	Value any
}

type Activation struct {
	Services []ProvidedService
	Effects  EffectScope
}

func (a *Activation) ProvideNamed(name string, value any) {
	a.Services = append(a.Services, ProvidedService{
		Name:  name,
		Value: value,
	})
}

type Component interface {
	Name() string
	Requirements() []string
	Activate(ctx *Context, act *Activation) error
}
