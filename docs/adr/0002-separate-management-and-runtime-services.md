# Keep Admin and Server as separate backend services

Web is the browser frontend, Admin is the management-plane backend and sole owner of configuration writes, and Server is the runtime data-plane backend optimized for reads. Admin and Server remain independently deployable and use separate read/write and read-only MySQL credentials; the first deployment omits a custom application Gateway because each caller already has one clear backend endpoint, while infrastructure ingress may still handle TLS and routing.
