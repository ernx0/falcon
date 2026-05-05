package runner

// builtinPathWordlist is a small, deliberately compact list of high-signal
// paths to fuzz against every live HTTP host. A bigger list lives at
// /usr/local/share/falcon/paths.txt inside the worker image — when it
// exists we prefer that, otherwise this in-binary list serves as a
// reliable fallback so ffuf always has something to chew on.
var builtinPathWordlist = []string{
	// admin / panels
	"admin", "administrator", "admin/login", "admin.php", "wp-admin",
	"phpmyadmin", "pma", "panel", "cpanel", "console", "dashboard",
	// auth
	"login", "signin", "sign-in", "logout", "register", "oauth",
	"auth", "auth/login", "auth/callback", "sso", "saml",
	// api
	"api", "api/v1", "api/v2", "api/v3", "api/docs", "api-docs",
	"api/users", "api/admin", "api/health", "api/status", "graphql",
	"swagger", "swagger.json", "swagger-ui", "openapi.json", "redoc",
	// secrets
	".env", ".env.local", ".env.production", ".git", ".git/config",
	".git/HEAD", ".gitignore", ".svn", ".DS_Store", ".aws/credentials",
	".htaccess", ".htpasswd", "config.json", "config.yml", "config.php",
	"web.config", "wp-config.php", "wp-config.php.bak",
	// backups & artifacts
	"backup", "backup.zip", "backup.tar.gz", "db.sql", "dump.sql",
	"archive.zip", "old", "old/", "backup/", ".bak", "test", "dev",
	// monitoring / debug
	"actuator", "actuator/health", "actuator/env", "metrics", "health",
	"healthz", "readyz", "status", "debug", "debug/pprof", "info",
	"phpinfo.php", "test.php",
	// common files
	"robots.txt", "sitemap.xml", "humans.txt", "security.txt",
	".well-known/security.txt", "crossdomain.xml", "clientaccesspolicy.xml",
	// uploads / static
	"upload", "uploads", "files", "static", "assets", "public",
	// app frameworks
	"server-status", "server-info", ".aws/", "_next/data", "wp-json",
	"wp-includes", "wp-content/uploads",
}
