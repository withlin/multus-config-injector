# multus-config-injector
Pod多网卡注入

## 背景

兼容来老项目,pod使用多网卡的方式，针对现在的容器云改造起来比较繁琐，所以需要用webhook的方式注入，省时省心。

## 原理

递归查找上层的cni配置，如果找到就不找了，如果第一个没找到cni的配置，一直往最顶层找，找到就给Pod打上多网卡的注解，找不到就放过。


