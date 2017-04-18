<?php
class Config
{
	protected $config;
	protected $filename;
	
	public function __construct($filename="../config/easywall.cfg") {
		$this->config = array();
		$this->filename = $filename;
		
		if ($this->readConfig($filename) === false) {
			return false;
		}
		return true;
    }
	
	protected function readConfig($filename) {
		$content = file($filename);
	
		foreach ($content as $line) {
			if (!ctype_space($line) && $line != '') {
				if (substr( $line, 0, 1 ) != "#") {
					$param = explode("=", $line);
					if (count($param) === 2) {
						$this->config[trim($param[0])] = trim($param[1]);
					}
				}
			}
		}
		
		if (count($this->config) > 0) {
			return true;
		}
		return false;
	}
	
	protected function writeConfig($filename) {
		$content = "";
	
		foreach ($this->config as $key => $value) {
			$content .= $key . "=" . $value . "\n";
		}
		
		if (file_put_contents($filename, $content) != false) {
			return true;
		}
		return false;
	}
	
	public function getValue($key) {
		if (isset($this->config)) {
			if ($this->config[$key] != null) {
				return $this->config[$key];
			}
		}
		return false;
    }
	
	public function setValue($key, $value) {
		if (ctype_space($value) || $value == '') {
			return $this->writeConfig($this->filename);
		}

		$this->config[$key] = $value;
		return $this->writeConfig($this->filename);
	}
}
?>