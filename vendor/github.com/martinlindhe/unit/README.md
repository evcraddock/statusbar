# About

[![Travis-CI](https://api.travis-ci.org/martinlindhe/unit.svg)](https://travis-ci.org/martinlindhe/unit)
[![codecov.io](https://codecov.io/github/martinlindhe/unit/coverage.svg?branch=master)](https://codecov.io/github/martinlindhe/unit?branch=master)
[![GoDoc](https://godoc.org/github.com/martinlindhe/unit?status.svg)](https://godoc.org/github.com/martinlindhe/unit)

Conversion of unit library for golang


# Installation

```
go get -u github.com/martinlindhe/unit
```


# General use

```go
ft := 1 * unit.Feet

fmt.Println("1 feet in meters = ", ft.Meters())
```

Please note the resulting precision is limited to the float64 type. A version of this lib using big decimal
numbers would be welcome, when [golang/issues/12127](https://github.com/golang/go/issues/12127) has been resolved.


# Temperature

Cannot be used to scale directly like the other units.
Instead, use the From* functions to create a Temperature type:

```go
f := unit.FromFahrenheit(100)

fmt.Println("100 fahrenheit in celsius = ", f.Celsius())
```


# License

Under [MIT](LICENSE)
