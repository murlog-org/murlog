FROM httpd:2.4

# Enable required modules and include murlog config.
# 必要なモジュールを有効化し、murlog 設定を読み込む。
RUN sed -i \
    -e 's/^[[:space:]]*#LoadModule cgid_module/LoadModule cgid_module/' \
    -e 's/^[[:space:]]*#LoadModule cgi_module/LoadModule cgi_module/' \
    -e 's/^[[:space:]]*#LoadModule rewrite_module/LoadModule rewrite_module/' \
    /usr/local/apache2/conf/httpd.conf
COPY httpd-murlog.conf /usr/local/apache2/conf/extra/httpd-murlog.conf
RUN echo 'Include conf/extra/httpd-murlog.conf' >> /usr/local/apache2/conf/httpd.conf

# Copy murlog CGI binary and SPA files.
# murlog CGI バイナリと SPA ファイルをコピー。
COPY murlog.cgi /usr/local/apache2/htdocs/murlog.cgi
COPY dist/ /usr/local/apache2/htdocs/dist/

# Seed .htaccess (normally auto-generated at runtime, but CGI needs it to boot).
# .htaccess を配置 (通常は起動時に自動生成されるが、CGI は起動にこれが必要)。
COPY htaccess /usr/local/apache2/htdocs/.htaccess

# Set ownership and permissions (simulate suEXEC user).
# パーミッション設定 (suEXEC ユーザーをシミュレート)。
RUN chmod 755 /usr/local/apache2/htdocs/murlog.cgi && \
    chown -R www-data:www-data /usr/local/apache2/htdocs/

EXPOSE 80
