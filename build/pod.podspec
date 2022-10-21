Pod::Spec.new do |spec|
  spec.name         = 'Gsdc'
  spec.version      = '{{.Version}}'
  spec.license      = { :type => 'GNU Lesser General Public License, Version 3.0' }
  spec.homepage     = 'https://github.com/sdcereum/go-sdcereum'
  spec.authors      = { {{range .Contributors}}
		'{{.Name}}' => '{{.Email}}',{{end}}
	}
  spec.summary      = 'iOS sdcereum Client'
  spec.source       = { :git => 'https://github.com/sdcereum/go-sdcereum.git', :commit => '{{.Commit}}' }

	spec.platform = :ios
  spec.ios.deployment_target  = '9.0'
	spec.ios.vendored_frameworks = 'Frameworks/Gsdc.framework'

	spec.prepare_command = <<-CMD
    curl https://gsdcstore.blob.core.windows.net/builds/{{.Archive}}.tar.gz | tar -xvz
    mkdir Frameworks
    mv {{.Archive}}/Gsdc.framework Frameworks
    rm -rf {{.Archive}}
  CMD
end
