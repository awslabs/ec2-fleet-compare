# ec2FleetCompare
Is a small / fast command-line tool which determine the instance types that fulfil your needs. Filter the output based on fleet size (both specific, or min number of instances), # of vcpu's, GB's of memory, network and disk requirements. The tool will compare all instance variations that meet your criteria and then output a table showing instance/hour cost and total cost per Month. 

ec2FleetCompare supports on-demand, spot and all variations of RI prices. Sorting of the output defaults to on-demand but can be changed via command-line parameter.

ec2FleetCompare can be used to look at cost comparisons on individual instances or a fixed number or instances but can also be used to find the cheapest options for a "fleet" of ec2 instances. As long as you know the total number of VCPS's or GB's or ram required across an entire fleet the tool will provide you the cheapest option to achieve this. This is especially useful when setting up a cluster of worker nodes on a ECS, Kubernetes or Mesos cluster for example.

# Download

Pre compiled binaries are available for Mac, Linux and Windows. Please see download links below:

[Linux Download] (https://s3-us-west-2.amazonaws.com/andy-gen/ec2FleetCompare/linux/ec2FleetCompare)  - (md5 c928053ef07bca2849001e5b8165d1bc)

[Mac Download] (https://s3-us-west-2.amazonaws.com/andy-gen/ec2FleetCompare/osx/ec2FleetCompare) - (md5 a0fe0603308069db4ebc376f0e2fa7db) 

[Windows Download] (https://s3-us-west-2.amazonaws.com/andy-gen/ec2FleetCompare/win/ec2FleetCompare.exe) - (md5 d165157d7235c76bf7162f493051e7c8)

Note: you may need to make executable i.e ```chmod 500 ./ec2FleetCompare``` or similiar for windows.

# Usage

To view help / option information. All options have defaults, the various options over-ride them.

```
./ec2FleetCompare --help
```
# Output

The command line tool will output a ASCII based table describing the instance types (and number of them if using for a fleet) that fulfil your criteria. This is sorted by default by on-demand pricing but can also be sorted via spot or RI pricing too.

```
+--------+-------------+------+-----------+-------+------------+---------+---------+-------------+-----------+----------+------------+--------+----------+
| # INST |    TYPE     | VCPU | VCPU FREQ |  MEM  |  NETWORK   | IS TYPE | IS SIZE | DEMAND/HOUR | SPOT/HOUR | SPOT SAV | DEMAND/MON | RI/MON | SPOT/MON |
+--------+-------------+------+-----------+-------+------------+---------+---------+-------------+-----------+----------+------------+--------+----------+
|      1 | c4.8xlarge  |   36 | 2.9 GHz   |  60.0 | 10 Gigabit | N/A     | N/A     | $1.68       | $0.26     | 84%      | $1,206     | $777   | $189     |
|      1 | c3.8xlarge  |   32 | 2.8 GHz   |  60.0 | 10 Gigabit | SSD     | 640 GB  | $1.68       | $0.41     | 75%      | $1,209     | $734   | $297     |
|      1 | cc2.8xlarge |   32 | 2.6 GHz   |  60.5 | 10 Gigabit | HDD     | 3360 GB | $2.00       | $0.26     | 87%      | $1,440     | $676   | $187     |
|      1 | m4.10xlarge |   40 | 2.4 GHz   | 160.0 | 10 Gigabit | N/A     | N/A     | $2.39       | $0.40     | 83%      | $1,723     | $1,019 | $288     |
|      1 | g2.8xlarge  |   32 | 2.6 GHz   |  60.0 | 10 Gigabit | SSD     | 240 GB  | $2.60       | $1.40     | 46%      | $1,872     | N/A    | $1,007   |
|      1 | r3.8xlarge  |   32 | 2.5 GHz   | 244.0 | 10 Gigabit | SSD     | 640 GB  | $2.66       | $0.26     | 90%      | $1,915     | $1,046 | $187     |
|      1 | cr1.8xlarge |   32 |           | 244.0 | 10 Gigabit | SSD     | 240 GB  | $3.50       | $0.40     | 89%      | $2,520     | $1,048 | $287     |
|      1 | m4.16xlarge |   64 | 2.3 GHz   | 256.0 | 20 Gigabit | N/A     | N/A     | $3.83       | $0.57     | 85%      | $2,757     | $1,631 | $408     |
|      1 | d2.8xlarge  |   36 | 2.4 GHz   | 244.0 | 10 Gigabit | HDD     | 8000 GB | $5.52       | $0.58     | 89%      | $3,974     | $1,994 | $418     |
|      1 | i2.8xlarge  |   32 | 2.5 GHz   | 244.0 | 10 Gigabit | SSD     | 6400 GB | $6.82       | $0.69     | 90%      | $4,910     | $2,107 | $494     |
|      1 | p2.8xlarge  |   32 |           | 488.0 | 10 Gigabit | N/A     | N/A     | $7.20       | $72.00    | -900%    | $5,184     | $3,392 | $51,840  |
|      1 | p2.16xlarge |   64 |           | 768.0 | 20 Gigabit | N/A     | N/A     | $14.40      | $144.00   | -900%    | $10,368    | $6,786 | $103,680 |
+--------+-------------+------+-----------+-------+------------+---------+---------+-------------+-----------+----------+------------+--------+----------+
```

# Example Usage

Find linux based ec2 instances with over 8GB ram and at least 4 VCPU's
```
./ec2FleetCompare -c 4 -m 8
```

Find cost of 70 c3.xlarge instances with a 3 year partial RI
```
./ec2FleetCompare -n 70 -i c3.xlarge -ri partial3
```

Find Windows based ec2 instances that have at least 1TB of SSD instance store disk available.
```
./ec2FleetCompare -os win -dt ssd -d 1024
```

Find Windows based ec2 instances that have Gigabit network interfaces, sorted by Spot pricing
```
./ec2FleetCompare -os win -nw gbit -s spot
```

Find cheapest fleet  that has a total VCPU capacity of 10000, with each instance at least having 32 VCPUS. Total memory capacity of at least 24TB with all nodes having Gigabit networking. Sorted by spot pricing.
```
./ec2FleetCompare -fc 10000 -c 32 -fm 24576 -nw gbit -s spot
```

Find cheapest fleet of i2 type type instances with a total memory cpacity of 24TB with each node having at least 3.2TB of SSD instance store disk available. Sorted by spot pricing.
```
./ec2FleetCompare -fm 24576 -dt SSD -d 3200 -i i2 -s spot
```

# Developing

This is written in [golang] (https://golang.org/). So you will need to download the GO compiler, set your ```GOPATH``` environment variable correctly and then install all the pre-req modules listed in the source file (```go get <package>```). 

