
# Kubernetes Deployment

The following guide walks through the steps to deploy the Vault Plugin within a Kubernetes environment.
The steps are divided into two sections they first covers the deployment of a Redis Enterprise cluster through the Redis Enterprise K8s Operator 
and the second covers the deployment of Vault and the Redis Vault Plugin.  Where appropriate supportive links are provided within each section.

Note: This guide is aimed at the provsioning of a development k8s based environment to execute the Vault Plugin.  
It is not a guide to provision a production grade environment. 



### Prerequisites

The following prerequisites have been used during the provisioning of Redis Enterprise and HashiCorp Vault on Kubernetes, (k8s).

* **k8s platform**
  * Docker Desktop has been used with 6 CPUs, 10GB Memory and 2GB Swap.  It was observed that 10GB of disk storage was used.
* **kubectl**
  * Installed and configured with access to k8s platform.
* **jq**
  * Used to extract required values from JSON returned by cli tools.
* **helm**
  * Used to provision HashiCorp Vault into k8s.  HashiCorp Vault chart supports Helm v3 and above.
  

## Deploying Redis Enterprise to K8s

The primary method to deploy a Redis Enterprise cluster to K8s is through the Redis Enterprise Operator.
Using the Operator is the most efficient way to deploy and maintain a cluster.

More information about the Redis Enterprise Operator can be found through the following link.
* [Redis Enterprise Kubernetes Operator-based Architecture](https://docs.redislabs.com/latest/platforms/kubernetes/concepts/operator)

### Operator

The Redis Enterprise Operator k8s configuration can be downloaded the RedisLabs GitHub account.  The following commands locate the latest version of the 
Operator, download the appropriate k8s YAML file and then apply the configuration through kubectl to the local k8s environment.

```sh
$ export VERSION=`curl --silent https://api.github.com/repos/RedisLabs/redis-enterprise-k8s-docs/releases/latest | grep tag_name | awk -F'"' '{print $4}'`
$ curl --silent -O https://raw.githubusercontent.com/RedisLabs/redis-enterprise-k8s-docs/$VERSION/bundle.yaml 
$ cat bundle.yaml | kubectl apply -f -

role.rbac.authorization.k8s.io/redis-enterprise-operator created
rolebinding.rbac.authorization.k8s.io/redis-enterprise-operator created
serviceaccount/redis-enterprise-operator created
Warning: apiextensions.k8s.io/v1beta1 CustomResourceDefinition is deprecated in v1.16+, unavailable in v1.22+; use apiextensions.k8s.io/v1 CustomResourceDefinition
customresourcedefinition.apiextensions.k8s.io/redisenterpriseclusters.app.redislabs.com created
deployment.apps/redis-enterprise-operator created
customresourcedefinition.apiextensions.k8s.io/redisenterprisedatabases.app.redislabs.com created
```
After the Operator has been successfully install to k8s CRDs representing the a cluster and database can be used.
The operator is now installed and ready for cluster and database resource definitions to be applied. 

### Cluster

After the Redis Enterprise Operator has been installed a cluster is created through the RedisEnterpriseCluster resource.  
The following command creates a cluster called `test-cluster` with a single node.

```sh
$ cat <<EOF | kubectl apply -f -
apiVersion: "app.redislabs.com/v1"
kind: "RedisEnterpriseCluster"
metadata:
  name: "test-cluster"
spec:
  nodes: 1
  redisEnterpriseNodeResources:
    limits:
      cpu: 1000m
      memory: 2Gi
    requests:
      cpu: 25m
      memory: 2Gi
EOF

redisenterprisecluster.app.redislabs.com/test-cluster created
```

A cluster will be present within the default namespace within the k8s environment.  A database can be provisioned within the cluster.


### Database

Once the Redis Enterprise Cluster has been created then a database can be created through a RedisEnterpriseDatabase resource.  
The following command creates a database called `mydb` with a role called `DB Member`.

```sh
$ cat <<EOF | kubectl apply -f -
apiVersion: app.redislabs.com/v1alpha1
kind: RedisEnterpriseDatabase
metadata:
  name: mydb
spec:
  memory: 100MB
  redisEnterpriseCluster:
    name: test-cluster
  rolesPermissions:
  - type: redis-enterprise
    role: "DB Member"
    acl: "Not Dangerous"
EOF

redisenterprisedatabase.app.redislabs.com/mydb created
```

The database will have been provisioned within the cluster and credentials a set of Administrator credentials will have been created.

### Service

Following the creation of the cluster and the database external k8s access can be achieved through a provioned service.  This will allow 
the Vault Plugin's tests to use the provisioned Redis Enterprise cluster.

The following command provisions a service to expose the cluster externally to k8s.

```sh
$ cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  labels:
    app: redis-enterprise
    redis.io/cluster: test-cluster
  name: external-access
spec:
  ports:
  - name: api
    port: 9443
    protocol: TCP
    targetPort: 9443
    nodePort: 30000
  selector:
    app: redis-enterprise
    redis.io/cluster: test-cluster
    redis.io/role: node
  sessionAffinity: None
  type: NodePort
EOF

service/test-cluster-service created
```

### Testing Cluster with Vault Plugin Tests

Before executing the Vault plugin tests the state of the cluster and the status of the database can be checked.
After the database has been provisioned the cluster will return a state of `running` and the database will report a state 
of `active`.

```sh
$ kubectl get redisenterpriseclusters.app.redislabs.com test-cluster --output jsonpath='{.status.state}'
Running

$ kubectl get redisenterprisedatabase.app.redislabs.com mydb --output jsonpath='{.status.status}'
active
```

Now the database is active the Vault Plugin tests can executes. For the tests to executed a set of credentials will need to be extracted from k8s. 
The following command will extract the username, password and node port and placed them in environment variable to be discovered by the executing tests.

```sh
$ export TEST_USERNAME=$(kubectl get secret test-cluster -o jsonpath='{.data.username}' | base64 -d)
$ export TEST_PASSWORD=$(kubectl get secret test-cluster -o jsonpath='{.data.password}' | base64 -d)
$ export TEST_DB_URL=https://localhost:$(kubectl get svc external-access --output jsonpath='{.spec.ports[].nodePort}')

$ make test
```

Alongside the execution of the tests the Redis Enterprise dashboard can be used to explore the cluster.  The following kubectl command 
can be used to forward the local port of 8443 to the cluster's open port 8443.  After establishing the port forward, a browser can be pointed to 
`http://localhost:8443`.  The previously mentioned username and password can be used to authenticate with the dashboard.

```shell
$ kubectl port-forward test-cluster-0 8443:8443
```

At this point in the setup a functioning Redis Enterprise cluster and database has been provision and verified through 
the Vault Plugin tests.

## Deploying Vault to K8s

HashiCorp Vault can operate both internally and externally to a k8s environment.  Additionally Vault is prepackaged with a 
number of Vault Plugins directly supported by HashiCorp.  The following sections will cover the provisoning of Vault within k8s 
and the setup of the Vault Plugin.  It is assumed that the user has provisioned Redis Enterprise using the first part of this guide.

### Vault

The preferred method to provision Vault within k8s is through the official HashiCorp Helm chart.  For this guide Vault will be provisioned 
into its own namespace and for the reminder of the guide the k8s context will be switched into a namespace called `vault`.

```sh
$ kubectl create namespace vault
$ kubectl config set-context --current --namespace=vault
```

Helm charts allow various degrees of guided customisation for the applications they provision.  For this guide the following overriding values 
will be used with the Vault helm chart.

The key takeaway from this overriding configuration is the location of the plugins directory.  This will be used later to register the Vault Plugin binary 
with Vault.

Note:  For the purpose of this guide the emptyDir volume is used, but a more appropriate volume should be used when multiple k8s nodes are used.
If the Vault pod is moved to another node then the content stored in this volume will be erased.

```sh
$ cat <<EOF>> override-values.yaml
server:
  standalone:
    config: |
      ui = true
      listener "tcp" {
        tls_disable = 1
        address = "[::]:8200"
        cluster_address = "[::]:8201"
      }
      storage "file" {
        path = "/vault/data"
      }
      plugin_directory = "/usr/local/libexec/vault"
  volumes:
    - name: plugins
      emptyDir: {}
  volumeMounts:
    - mountPath: /usr/local/libexec/vault
      name: plugins
EOF
```

The following command will instruct Helm to provision Vault using the previously generated YAML file.

```shell
$ helm install vault hashicorp/vault --version 0.9.1 --values ./override-values.yaml
```

Once Vault is deployed it must be initialised and unsealed.  The following commands will initialise Vault and extract 
the root token and unseal key.  Finally Vault is unsealed using the key and Vault's status can be checked.

```shell
$ kubectl exec vault-0 -- vault operator init -key-shares=1 -key-threshold=1 -format=json > cluster-keys.json
$ VAULT_UNSEAL_KEY=$(cat cluster-keys.json | jq -r ".unseal_keys_b64[]")
$ kubectl exec vault-0 -- vault operator unseal $VAULT_UNSEAL_KEY

$ kubectl exec -n vault vault-0 -- vault status
```

Vault is now ready to accept the Redis Enterprise Vault Plugin.

### Vault Plugin

After the Vault instance has been unsealed the Redis Enterprise Vault Plugin can be installed.  Firstly the plugin binary is built
and placed in the `bin` directory.  There will be two binaries the first called `vault-plugin-database-redisenterprise` and
the second called `vault-plugin-database-redisenterprise_linux_amd64`.  The `vault-plugin-database-redisenterprise_linux_amd64` 
binary is copied to the vault mounted volume.

```shell
$ make build
$ kubectl cp ../bin/vault-plugin-database-redisenterprise_linux_amd64 vault-0:/usr/local/libexec/vault
```

The root token is needed to log into the Vault instance and this is present in the `cluster-keys.json` file created earlier.
It can be extract through the following command.

```shell
$ cat cluster-keys.json | jq -r ".root_token"
```

To register the vault plugin first open up a terminal prompt inside the vault container and use the `vault login` command
together with the root token.  From this point on the following commands will be executed inside the Vault container.

```sh
$ kubectl exec --stdin=true --tty=true vault-0 -- /bin/sh
$ vault login
```

The vault plugin binary must be registered in the Vault catalog before it can be used.  To do this the SHA of the binary 
must be known and supplied with the location of the binary. A success message will indicate that the plugin is registered.

NOTE:  The success message only gives confirmation the plugin is registered, but does not verify that the SHA value matches the binary.  
If this is wrong then Vault will communicate this through later commands.

```shell
$ export REDIS_VAULT_PLUGIN_SHA=$(sha256sum /usr/local/libexec/vault/vault-plugin-database-redisenterprise_linux_amd64 | awk '{ print $1 }')
$ vault write sys/plugins/catalog/database/redisenterprise-database-plugin command=vault-plugin-database-redisenterprise_linux_amd64 sha256=$REDIS_VAULT_PLUGIN_SHA
Success! Data written to: sys/plugins/catalog/database/redisenterprise-database-plugin
```

Enable to database secrets engine

```shell
$ vault secrets enable database
Success! Enabled the database secrets engine at: database/
```

Next the registered plugin is configured with the details to locate and authenticate with the Redis Enterprise Cluster.

Note:  For the purpose of this guide the admin username and password can be used to register the plugin within Vault. 
It is advised to create an additional user to represent Vault following the [User Management](https://docs.redislabs.com/latest/rs/administering/access-control/) guide.

```shell
$ vault write database/config/redis-mydb plugin_name="redisenterprise-database-plugin" url="https://external-access.default.svc.cluster.local:9443" allowed_roles="*" database=mydb username=<PROVISIONED USERNAME> password=<PROVISIONED PASSWORD>
```

Following a successful authentication with the Redis Enterprise cluster Vault must be configured with a creation statement to use through the Vault Plugin.

```shell
$ vault write database/roles/mydb db_name=redis-mydb creation_statements="{\"role\":\"DB Member\"}" default_ttl=3m max_ttl=5m
```

The final step is to verify the registration and configuration of the plugin through Vault.  The following `vault read` command will 
generate a new user against the Redis Enterprise cluster.

```shell
$ vault read database/creds/mydb
```
