"""this is the setup module for easywall"""
from setuptools import setup


def readme():
    """include the readme as long_description"""
    with open('README.md') as handler:
        return handler.read()


setup(
    name='easywall',
    version='0.0.4',
    description='Software for simple control of Linux firewalls via configuration files written in Python',
    long_description=readme(),
    classifiers=[
        'Development Status :: 3 - Alpha',
        'License :: OSI Approved :: GNU General Public License v3 or later (GPLv3+)',
        'Programming Language :: Python :: 3.7',
        'Topic :: System :: Networking :: Firewalls',
    ],
    keywords="firewall iptables linux framework easywall",
    url='https://github.com/jp1337/easywall',
    author='JP',
    author_email='12394156+jp1337@users.noreply.github.com',
    license='GPL-3.0',
    packages=['easywall'],
    install_requires=[
        'watchdog',
    ],
    include_package_data=True,
    zip_safe=False,
    setup_requires=[
        'pytest-runner',
    ],
    tests_require=[
        'pytest-cov',
        'pytest-pythonpath',
        'pytest',
        'coverage'
    ]
)
