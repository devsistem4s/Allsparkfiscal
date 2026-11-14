FROM php:8.3-cli-bookworm AS base
RUN apt-get update && apt-get install -y --no-install-recommends libxml2-dev libpng-dev libjpeg62-turbo-dev libfreetype6-dev libonig-dev libzip-dev unzip git ca-certificates && docker-php-ext-configure gd --with-freetype --with-jpeg && docker-php-ext-install -j2 soap gd mbstring zip && rm -rf /var/lib/apt/lists/*
COPY --from=composer:2 /usr/bin/composer /usr/bin/composer
WORKDIR /app
COPY composer.json composer.lock* ./
RUN composer install --no-dev --prefer-dist --no-interaction --no-progress --optimize-autoloader
COPY src ./src
COPY public ./public
COPY tests ./tests
ENV PHP_CLI_SERVER_WORKERS=4
USER 1000:1000
CMD ["php","-d","display_errors=0","-d","log_errors=1","-d","memory_limit=256M","-S","0.0.0.0:9000","public/index.php"]
