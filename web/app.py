import flask

app = flask.Flask(__name__)


@app.route('/')
def index():
    return flask.render_template('index.html')


@app.route('/cakes')
def cakes():
    return 'Yummy cakes!'


@app.route('/hello/<name>')
def hello(name):
    return flask.render_template('page.html', name=name)


if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
