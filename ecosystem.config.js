module.exports = {
    apps: [{
        name: 'thg-scraper',
        script: './scraper',
        cwd: '/app',

        // Auto-restart
        autorestart: true,
        watch: false,
        max_restarts: 10,
        restart_delay: 5000,

        // Memory limit (restart if exceeded)
        max_memory_restart: '2G',

        // Logging
        log_file: '/app/data/logs/combined.log',
        out_file: '/app/data/logs/out.log',
        error_file: '/app/data/logs/error.log',
        log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
        merge_logs: true,

        // Environment
        env: {
            NODE_ENV: 'production',
            WEB_PORT: 8080,
            DB_PATH: 'data/scraper.db',
            PROFILE_DIR: 'data/profiles',
            MAX_WORKERS: 1,
            SCAN_INTERVAL_MIN: 30,
            BACKUP_ENABLED: 'true',
        }
    }]
};
