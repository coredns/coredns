package kubernetes

func (k *Kubernetes) namespace(n string) bool {
	ns, err := k.APIConn.GetNamespaceByName(n)
	if err != nil {
		return false
	}
	return ns.ObjectMeta.Name == n
}
