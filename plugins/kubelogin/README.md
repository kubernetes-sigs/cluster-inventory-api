# Authenticate with an AKS cluster using its cluster profile via the `kubelogin` exec plugin

This code example explains how to authenticate with an AKS cluster using its cluster profile via
the `kubelogin` exec plugin.

`kubelogin` is a Kubernetes credential exec plugin implementing authentication methods supported
on Azure. For more information, see the `kubelogin` [documentation](https://azure.github.io/kubelogin/index.html)
and [source code](https://github.com/Azure/kubelogin).

`kubelogin` supports a variety of authentication methods. This example uses the non-interactive 
Microsoft Entra ID (Azure AD) federated identity based authentication (also known as workload identity
based authentication). This is a method that works best for a multi-cluster application (e.g., ArgoCD, Kueue)
running on an AKS cluster to use the token issued by the Kubernetes API server itself (AKS OIDC issuer) to
access another Kubernetes cluster. For more information, refer to the Azure documentation. See
`kubelogin` [introduction page](https://azure.github.io/kubelogin/index.html) for a list of other authentication
methods supported by `kubelogin`.

## How this code example works

The code example assumes that:

* federation authentication has been set up successfully;
* the application running the code snippet has acquired its federated identity token (mounted at a file path);
* the application has retrieved the cluster profile that describes the cluster it would like to access.

The application first specifies an authentication provider that uses the `kubelogin` exec plugin and
the federated identity based authentication method.

It then passes a cluster profile to the provider so that it can prepare a `KUBECONFIG` file that a Kubernetes
client can make use of. In the cluster profile a multi-cluster management platform has included
the information a client needs to access the cluster, specifically:

* the name and the API server address of the cluster;
* the CA data of the cluster;
* the tenant ID of the target cluster, plus the client ID and authority host to use for authentication.

Note that with the `clusterprofiles.multicluster.x-k8s.io/exec/additional-args` and
`clusterprofiles.multicluster.x-k8s.io/exec/additional-envs` extensions, the cluster profile has explicitly
specified how the tenant ID, client ID, and the authority host information should be passed
to the `kubelogin` exec plugin (as CLI arguments and/or environment variables). The provider
has been set to accept such overrides.
