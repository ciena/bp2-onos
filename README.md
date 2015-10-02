# ONOS 1.3 Microservice [![bp onos](http://img.shields.io/badge/microservice-blueplanet-blue.svg)]() [![bp onos](http://img.shields.io/badge/microservice-kubernetes-blue.svg)]()
Provides ON.lab's ONOS 1.3 SDN controller as a blueplanet and kubernetes compatible microservice that supports dynamic scaling utilizing the platforms' clustering capabilities.

### What it provides?
This microservice provides basic ONOS 1.3 as a cluster-able microservice and exposes publishes both the ONOS REST API into the blueplant platform environement under then interface name `onos` on port `8181`. Additionally in this repository is a sample blueplanet solution file, `onoscluster.yml` the create a 3 node cluster ONOS deployment. Once started the number of ONOS nodes can be scaled up and down using the blueplanet solution manager command `solution_app_scale`.

Also provided in this repository are some shell utility functions in `util.env`. The functions in this file are convenience routines to help access to log files, etc. that are part of the running solution. These may be rather specific to my environment including my docker deployment. If nothing else it may give you hints at things to look at.

### How it works
This ONOS microservice is based on ONOS 1.3 (drake) release. This release of ONOS does not support dynamic cluster scaling, which is a key feature of the blueplanet platform. As such, something needed to be done to make a version of ONOS that doesn't support dynamic clustering support dynamic clustering.

To accomplish this task two components were build: wrapper and hook.

- **wrapper** - the wrapper is the main process of the microservice [docker] container. this process starts an instance of ONOS and then will update its cluster configuration and kill / restart the ONOS process such that ONOS will accept the updated cluster information.
- **hook** - the hook is the blueplanet hook function that handles the various messages from the blueplanet platform including `southbound-update`, `heartbeat`, `peer-state`, and `peer-update`. For this solution the the hooks associated with clustering (`heartbeat`, `peer-update`, and `peer-status`) are serviced. when the clustering information (list of IP addresses in the cluster) is updated from the platform the hook writes the new cluster configuration to a known location and then triggers the wrapper to restart the ONOS process.

### Taking it for a drive ![bp](bp.png)

#### Building applications
To build the project just issue the `make all` command. This will build the wrapper and hook applications and then bundle everything into a docker image tagged with the name `cyan/onos:1.3`. The `1.3` represents the version of ONOS on which this is based.

#### Building solution
To build the solution the blueplanted solmaker is used. How to install this application is documented on the cyan/ciena web sites and so won't be repeated here. In general the command to execute is:

    solmaker build --tag=13 onoscluster.yml

#### Starting the ONOS cluster
Assuming you already have the blueplant platform installed and running, including the `solutionmanager`, `etcd`, `discod`, and `haproxy`, the next step would be to start the `onoscluster` solution. From the solution manager CLI (`smcli`) simply deploy the solution:

    (Cmd) solution_deploy cyan.onoscluster:13

This will start a 3 node ONOS cluster that will be accessible, both the UI and API, via the haproxy as `http://<haproxy-ip>/onos/...`.

_**Note**_ - _as cluster membership changes the ONOS instances are killed and restarted. This means that it can take seconds or minutes for the cluster to converge and be ready to service requests; depending on the hardware for the docker server._

#### Scaling solution
To scale the solution, up or down, the solution manager CLI (`smcli`) can be used:

    (Cmd) solution_app_scale cyan.onoscluster:13 bp2-onos <# instances>

The number of instances should be 3 or greater. Thus if you want to scale up the number of instance from the default you specify 5 as the number of the instanes. Once the instances are scaled up, you can scale down by entering a smaller number.

_**Note**_ - _as cluster membership changes the ONOS instances are killed and restarted. This means that it can take seconds or minutes for the cluster to converge and be ready to service requests; depending on the hardware for the docker server._

#### Shutting it down
To shutdown the cluster you use the service manager CLI (`smcli`) to undeploy the solution:

    (Cmd) solution_undeploy cyan.onoscluster:13

### At the helm, using it with kubernetes ![k8s](k8s.png)
The docker image build from this project supports both the BluePlanet and the kubernetes platforms. The instructions that follow assume you have an operational kubernetes cluster and that the `minions` associated with that cluster have access to the docker image produced from this build.

#### Create the kubernetes ONOS service
A service description file for the ONOS component can be found in this project as `k8s/onos-service.yaml`. To create the ONOS service, which creates a durable IP address through which clients can access ONOS, with the following command:

```
kubectl create -f k8s/onos-service.yaml
```

#### Create the kubernetes replication controller
A replication controller file for the ONOS component can be found in this project as `k8s/onos-rc.yaml`. By default this replication controller creates a single instance of ONOS. This can be scaled up with the `kubectl scale` command which is described later. The replication controller is installed with the following command:

```
kubectl create -f k8s/onos-rc.yaml
```

#### Accessing ONOS
As defined in the `k8s/onos-service.yaml` file, ONOS is exposed on port `8181` on the virtual IP on which the service is exposed. The virtual IP address can be seen by issueing the following command:

```
kubectl get svc
```

#### Scaling the solution
The number of ONOS instances is scaled using the `kubectl scale` command as depicted by the following command that scales the number of ONOS instances in the cluster to 3:

```
kubectl scale rc onos --replicas=3
```
_**Note**_ - _as cluster membership changes the ONOS instances are killed and restarted. This means that it can take seconds or minutes for the cluster to converge and be ready to service requests; depending on the hardware for the docker server._
