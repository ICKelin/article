# golang slice的实现以及常见的坑
go当中slice是常用的数据结构，这篇文章主要记录我在使用slice过程当中遇到的一个坑，然后通过遇到的坑去慢慢一步一步看go的slice的实现。

本文先写了两个在使用slice当中容易出现的两个错误，然后通过这两个错误之后，带着问题去看golang的slice的一些实现发现。

## slice易错点
**slice作为函数参数**
slice作为函数参数时，我们会思考一个问题，就是如果在调用的函数里面对slice进行修改，函数返回之后会不会影响调用方的slice。我的答案是可能会，也可能不会

这个问题需要分两种情况进行对待

- 被调用方改变数据，但是不对slice进行新增元素
- 被调用的函数对slice进行append操作，再细分又可以分为append之后是否进行扩容两种情况

针对第一种情况，编写以下测试程序:

```
package main

import (
        "fmt"
)

func main() {
        var slice = []int{1, 2, 3}
        fmt.Printf("before updateSlice: %v\n", slice)
        updateSlice(slice)
        fmt.Printf("after udpateSlice: %v\n", slice)
}

func updateSlice(src []int) {
        src[0] = 10
}
```
可以看到，在调用updateSlice之前和updateSlice之后，slice是有发生过改变了的，**所以这个测试说明会调用之后slice发生了改变了**

针对第二种情况，还要再分两种情况进行考虑，一种情况是append进行了扩容，一种情况是append没进行扩容

编写以下测试程序

```
package main

import (
        "fmt"
)


func main() {
	var slice = []int{1, 2, 3}
	// 进行扩容
	fmt.Println("slice扩容")
	fmt.Printf("before: len: %d, cap: %d %v\n", len(slice), cap(slice), slice)
	appendSlice(slice)
	fmt.Printf("after: len: %d, cap: %d %v\n", len(slice), cap(slice), slice)

	// 未进行扩容
	var slice1 = make([]int, 0, 2)
	slice1 = append(slice1, 1)

	fmt.Println("slice未扩容")
	fmt.Printf("before: len: %d, cap: %d %v\n", len(slice1), cap(slice1), slice1)
	appendSlice1(slice1)
	fmt.Printf("after: len: %d, cap: %d %v\n", len(slice1), cap(slice1), slice1)

}

func appendSlice(src []int) {
	src = append(src, 100)
	fmt.Printf("calling: len: %d, cap: %d %v\n", len(src), cap(src), src)
}

func appendSlice1(src []int) {
	src = append(src, 1001)
}

```

可以看到，不管有没有进行扩容，appendSlice之后都不会改变原来的slice输出结果，我这里说的是没有改变到原来的值，但是并不表示没有被影响到，实际上没扩容的场景是有影响到了的，只是由于slice的长度限制，没有显示出来而已。

通过以下程序可以将后面添加的1001这个值找出来，当然实际使用不建议写这样的代码。
```
type goSlice struct {
	array unsafe.Pointer
	len   int
	cap   int
}

func realSlice(src []int) {
	goslice := (*goSlice)(unsafe.Pointer(&src))
	goslice.len += 1
	fmt.Println(src)
}

```

**子切片**
在使用切片时，可能只用到一部分数据，这时候通常考虑子切片，使用子切片问题又来了——使用子切片会不会对原来的切片有影响，我的答案也是可能会有，也可能没有。

子切片用法类似的又分成两种情况：

- 使用子切片时，修改切片的值
- 使用子切片append元素，再细分又可以分为append导致扩容与否两种情况

首先来看第一种情况，在使用子切片时，修改切片元素

```
package main

import "fmt"

func main() {
	slice := []int{1, 2, 3, 4, 5}
	sub := slice[2:3]
	fmt.Printf("sub: len: %d cap: %d\n", len(sub), cap(sub))

	sub[0] = 100
	fmt.Println(sub, slice)
}

```

根据输出结果，可以看到slice是发生了改变，而且sub子切片的cap是3，这个是意想不到的，后续会对其进行说明，这个实验证明了一点，那就是修改子切片的值时，会改到原来切片的值。

接下来看第二中情况，第二种情况又分为子切片append之后未扩容和扩容两种情况

未扩容:
```
package main

import "fmt"

func main() {
	slice := []int{1, 2, 3, 4, 5}
	sub := slice[2:3]
	fmt.Printf("sub: len: %d cap: %d\n", len(sub), cap(sub))
	sub = append(sub, 100)
	fmt.Printf("sub: len: %d cap: %d\n", len(sub), cap(sub))
	fmt.Println(sub, slice)
}

```
这个实验的结果跟之前得到的结果是一致的。

扩容:

```
package main

import "fmt"

func main() {
	slice := []int{1, 2, 3, 4, 5}
	sub := slice[4:5]
	fmt.Printf("sub: len: %d cap: %d\n", len(sub), cap(sub))
	sub = append(sub, 100)
	fmt.Printf("sub: len: %d cap: %d\n", len(sub), cap(sub))
	fmt.Println(sub, slice)
}

```

这个实验的结果跟之前的两个实验得到的结果不一样。

到这里我就在想，如果不去了解slice的底层内存布局，以及slice是如何进行扩容的，可能对上面问题理解得都不是很透。

## slice扩容
go标准库runtime/slice.go包含slice的内存布局以及扩容机制的实现，关于slice的赋值，子切片等应该是编译器支持的，在这个文件下找不到相关的代码。

整个growslice的实现如下（已剔除部分代码）

```
func growslice(et *_type, old slice, cap int) slice {
    // 扩容计算，计算新slice的容量大小
    // 当旧的容量<1024时，容量翻倍
    // 否则，容量按照1/4的比例进行增长，直到容量满足条件
	newcap := old.cap
	doublecap := newcap + newcap
	if cap > doublecap {
		newcap = cap
	} else {
		if old.len < 1024 {
			newcap = doublecap
		} else {
			for 0 < newcap && newcap < cap {
				newcap += newcap / 4
			}
			if newcap <= 0 {
				newcap = cap
			}
		}
	}

    // 重新计算容量大小
	var overflow bool
	var lenmem, newlenmem, capmem uintptr
	switch {
	case et.size == 1:
		lenmem = uintptr(old.len)
		newlenmem = uintptr(cap)
		capmem = roundupsize(uintptr(newcap))
		newcap = int(capmem)
	case et.size == sys.PtrSize:
		lenmem = uintptr(old.len) * sys.PtrSize
		newlenmem = uintptr(cap) * sys.PtrSize
		capmem = roundupsize(uintptr(newcap) * sys.PtrSize)
		newcap = int(capmem / sys.PtrSize)
	case isPowerOfTwo(et.size):
		var shift uintptr
		if sys.PtrSize == 8 {
			// Mask shift for better code generation.
			shift = uintptr(sys.Ctz64(uint64(et.size))) & 63
		} else {
			shift = uintptr(sys.Ctz32(uint32(et.size))) & 31
		}
		lenmem = uintptr(old.len) << shift
		newlenmem = uintptr(cap) << shift
		capmem = roundupsize(uintptr(newcap) << shift)
		newcap = int(capmem >> shift)
	default:
		lenmem = uintptr(old.len) * et.size
		newlenmem = uintptr(cap) * et.size
		capmem, overflow = math.MulUintptr(et.size, uintptr(newcap))
		capmem = roundupsize(capmem)
		newcap = int(capmem / et.size)
	}

	var p unsafe.Pointer
	if et.ptrdata == 0 {
		p = mallocgc(capmem, nil, false)
		memclrNoHeapPointers(add(p, newlenmem), capmem-newlenmem)
	} else {
		// Note: can't use rawmem (which avoids zeroing of memory), because then GC can scan uninitialized memory.
		p = mallocgc(capmem, et, true)
		if lenmem > 0 && writeBarrier.enabled {
			bulkBarrierPreWriteSrcOnly(uintptr(p), uintptr(old.array), lenmem)
		}
	}
	memmove(p, old.array, lenmem)

	return slice{p, old.len, newcap}
}

```
可以看到append一开始的扩容调整机制是：

- 如果旧的容量小于1024，那么容量就进行*2
- 否则，容量*1.25

除了新容量调整之外，在新内存分配的时候，还需要去适配golang的mallocgc内存分配算法，所以不仅仅是按照以上策略进行分配这么简单，这里又会涉及内存分配相关的算法，又会是个很大的话题。

最后两条语句
```
	memmove(p, old.array, lenmem)

	return slice{p, old.len, newcap}
```
很清楚的在告诉我们，slice扩容需要对内存进行一次拷贝，将旧的array数据全部拷贝到p指向的内存当中，**并重新生成slice结构**

所以说，slice在不进行扩容时，所有操作都反应在底层的array指针上，如果进行扩容了，那么扩容之后就完全是一个新的slice，无论是len，cap还是array字段，都是新的，对新的array字段进行操作不会影响到原本的slice的array字段。

那么我们可以开始解释之前的slice作为函数参数和子切片这两个问题了：

- 作为函数参数的时候，虽然传递的是slice结构，但是底层array指向的是同一段地址空间。

如果在被调用的函数没有进行扩容，在被调用函数当中修改切片内容，最终反应到修改array指向的内存空间的数据。

如果在被调用的函数进行了append操作，如果没有扩容，会影响到array指向的内存的内容，但是由于调用方的len字段限制住了访问，所以这时候使用array是感知不到数据的变化的。

如果在被调用方进行了append操作，并且造成扩容，会开辟一个新的slice，并且将老的slice的数据复制一份过去，新slice的array字段指向的内存与老的slice的不一样，append的数据发生在新的slice上，所以不会影响到老的slice

- 作为子切片时，array指向的都是同一段地址空间的不同区域，所以修改子切片会造成切片的修改，但是如果子切片进行了扩容，那么和切片作为函数参数一样，重新分配了内存，后续对子切片的操作与切片本身无关。

所以他们的主要矛盾都在与是否进行了扩容上。
